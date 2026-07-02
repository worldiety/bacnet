package client

import (
	"context"
	"fmt"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
	"github.com/worldiety/bacnet/encoding"
)

// propObjectList is the object-list property identifier (ASHRAE 135).
const propObjectList types.PropertyIdentifier = 76

// ReadOptions configures a ReadProperty call.
type ReadOptions struct {
	// ArrayIndex selects a single array element (0 = the element count for
	// array properties). Nil reads the whole property.
	ArrayIndex *uint32
}

// ReadOption mutates ReadOptions.
type ReadOption func(*ReadOptions)

// AtIndex reads a single array element at the given index.
func AtIndex(i uint32) ReadOption {
	return func(o *ReadOptions) { o.ArrayIndex = &i }
}

// ReadProperty reads a single property from an object on the target device and
// returns the decoded value. For a device-ID target the address is resolved
// first via Who-Is.
func (c *Client) ReadProperty(ctx context.Context, target Target, obj Object, pid types.PropertyIdentifier, opts ...ReadOption) (PropertyValue, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return PropertyValue{}, err
	}
	var o ReadOptions
	for _, opt := range opts {
		opt(&o)
	}
	return c.readProperty(ctx, dst, obj.OID(), pid, o.ArrayIndex)
}

// readProperty performs a single ReadProperty against an already-resolved
// address and decodes the value.
func (c *Client) readProperty(ctx context.Context, dst netprim.Address, oid types.ObjectIdentifier, pid types.PropertyIdentifier, index *uint32) (PropertyValue, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.requestBudget())
	defer cancel()

	req, err := apdu.NewReadPropertyRequest(oid, pid, index)
	if err != nil {
		return PropertyValue{}, err
	}
	ack, err := c.apduClient().ReadProperty(reqCtx, dst, req)
	if err != nil {
		return PropertyValue{}, err
	}
	return decodeValue(ack.PropertyValue), nil
}

// decodeValue turns raw application-tagged bytes into a PropertyValue, keeping
// the raw bytes when decoding fails so callers can still recover the content.
func decodeValue(raw []byte) PropertyValue {
	if len(raw) == 0 {
		return PropertyValue{RawBytes: raw}
	}
	val, _, err := encoding.DecodeApplicationValue(raw, 0)
	if err != nil {
		return PropertyValue{RawBytes: raw}
	}
	return PropertyValue{Raw: val, RawBytes: raw}
}

// ObjectListOptions configures ReadObjectList.
type ObjectListOptions struct {
	// Device overrides the Device object instance whose object-list is read.
	// It is only consulted for address targets; for device-ID targets the
	// resolved instance is used. A negative value means "unset".
	Device int
	// Limit stops enumeration after this many objects (0 = no limit).
	Limit int
	// OnError, if set, is called for each element that fails to read instead of
	// aborting; return nil to continue. If nil, the first element error aborts.
	OnError func(index uint32, err error) error
}

// ReadObjectList reads a device's object-list and returns every object it
// contains. The list is read element-by-element (index 0 gives the count, then
// each index in turn) rather than in a single large read — gentle on slow MS/TP
// segments and robust against segmentation limits. Pacing (if configured) is
// applied between element reads.
//
// The object-list lives on the Device object. For a device-ID target the
// resolved instance is addressed. For an address target, opts.Device selects
// the instance; if unset it falls back to 4194303 (the reserved
// "unconfigured / this device" instance many devices accept as a self-reference).
func (c *Client) ReadObjectList(ctx context.Context, target Target, opts ObjectListOptions) ([]Object, error) {
	dst, instance, err := c.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}

	deviceInstance := uint32(4194303)
	switch {
	case target.isID:
		deviceInstance = instance
	case opts.Device >= 0:
		deviceInstance = uint32(opts.Device)
	}
	deviceOID, err := types.NewObjectIdentifier(types.ObjectTypeDevice, deviceInstance)
	if err != nil {
		return nil, fmt.Errorf("invalid device instance: %w", err)
	}

	count, err := c.readArrayCount(ctx, dst, deviceOID, propObjectList)
	if err != nil {
		return nil, fmt.Errorf("read object-list length of %s: %w", objectFromOID(deviceOID), err)
	}
	if opts.Limit > 0 && uint32(opts.Limit) < count {
		count = uint32(opts.Limit)
	}

	objects := make([]Object, 0, count)
	for i := uint32(1); i <= count; i++ {
		oid, err := c.readObjectListElement(ctx, dst, deviceOID, i)
		if err != nil {
			if opts.OnError != nil {
				if herr := opts.OnError(i, err); herr != nil {
					return objects, herr
				}
				c.pace(ctx)
				continue
			}
			return objects, fmt.Errorf("read object-list[%d]: %w", i, err)
		}
		objects = append(objects, objectFromOID(oid))
		c.pace(ctx)
	}
	return objects, nil
}

// readArrayCount reads element 0 of an array property, which is the number of
// elements in the array.
func (c *Client) readArrayCount(ctx context.Context, dst netprim.Address, oid types.ObjectIdentifier, pid types.PropertyIdentifier) (uint32, error) {
	zero := uint32(0)
	v, err := c.readProperty(ctx, dst, oid, pid, &zero)
	if err != nil {
		return 0, err
	}
	n, ok := v.Uint()
	if !ok {
		return 0, fmt.Errorf("array length has unexpected type")
	}
	return n, nil
}

// readObjectListElement reads element index (1-based) of object-list.
func (c *Client) readObjectListElement(ctx context.Context, dst netprim.Address, deviceOID types.ObjectIdentifier, index uint32) (types.ObjectIdentifier, error) {
	v, err := c.readProperty(ctx, dst, deviceOID, propObjectList, &index)
	if err != nil {
		return 0, err
	}
	oid, ok := v.ObjectID()
	if !ok {
		return 0, fmt.Errorf("object-list element has unexpected type")
	}
	return oid, nil
}

// ReadObjectNames reads the object-name of each given object on the target
// device, returning a map from object to name. Objects whose name cannot be
// read are omitted from the map (they are not treated as fatal). Pacing (if
// configured) is applied between reads.
func (c *Client) ReadObjectNames(ctx context.Context, target Target, objs []Object) (map[Object]string, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	names := make(map[Object]string, len(objs))
	for _, obj := range objs {
		v, err := c.readProperty(ctx, dst, obj.OID(), types.PropertyIdentifierObjectName, nil)
		if err == nil {
			if s, ok := v.Text(); ok {
				names[obj] = s
			}
		}
		c.pace(ctx)
	}
	return names, nil
}

// PropertyResult pairs a property identifier with its read result. Value is
// only meaningful when Err is nil.
type PropertyResult struct {
	Property types.PropertyIdentifier
	Value    PropertyValue
	Err      error
}

// ReadProperties reads several properties of one object, one request per
// property (gentle on slow lines and robust against unsupported properties). It
// resolves the target once. Per-property errors are captured in the results
// rather than aborting. Pacing (if configured) is applied between reads.
func (c *Client) ReadProperties(ctx context.Context, target Target, obj Object, pids []types.PropertyIdentifier) ([]PropertyResult, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	results := make([]PropertyResult, 0, len(pids))
	for _, pid := range pids {
		v, rerr := c.readProperty(ctx, dst, obj.OID(), pid, nil)
		results = append(results, PropertyResult{Property: pid, Value: v, Err: rerr})
		c.pace(ctx)
	}
	return results, nil
}

// DefaultProperties returns a reasonable set of properties to display for an
// object, tailored a little by object type. Intended for a "show me this
// object" overview.
func DefaultProperties(obj Object) []types.PropertyIdentifier {
	switch obj.Type {
	case types.ObjectTypeDevice:
		return []types.PropertyIdentifier{
			types.PropertyIdentifierObjectName,
			types.PropertyIdentifierObjectType,
			types.PropertyIdentifierDescription,
			types.PropertyIdentifierVendorName,
			mustProp("vendor-identifier"),
			mustProp("model-name"),
			mustProp("firmware-revision"),
			types.PropertyIdentifierApplicationSoftwareVersion,
			types.PropertyIdentifierProtocolVersion,
			types.PropertyIdentifierProtocolRevision,
			mustProp("system-status"),
			mustProp("location"),
		}
	default:
		return []types.PropertyIdentifier{
			types.PropertyIdentifierObjectName,
			types.PropertyIdentifierObjectType,
			types.PropertyIdentifierDescription,
			types.PropertyIdentifierPresentValue,
			types.PropertyIdentifierStatusFlags,
			types.PropertyIdentifierUnits,
			mustProp("out-of-service"),
			mustProp("reliability"),
		}
	}
}

// mustProp looks up a known property name from the table. All names passed here
// are present in the table, so the lookup always succeeds.
func mustProp(name string) types.PropertyIdentifier {
	pid, _ := PropertyByName(name)
	return pid
}

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
//
// A property body may hold a single application value or a sequence of them (a
// list-valued property such as object-list). Every leading application value is
// decoded into PropertyValue.Values, with Raw set to the first. Decoding stops
// at the first byte that is not a plain application value (e.g. a
// context/constructed segment); anything decoded so far is retained along with
// the full RawBytes.
func decodeValue(raw []byte) PropertyValue {
	if len(raw) == 0 {
		return PropertyValue{RawBytes: raw}
	}
	var vals []encoding.ApplicationValue
	off := 0
	for off < len(raw) {
		v, next, err := encoding.DecodeApplicationValue(raw, off)
		if err != nil || next <= off {
			break
		}
		vals = append(vals, v)
		off = next
	}
	if len(vals) == 0 {
		return PropertyValue{RawBytes: raw}
	}
	return PropertyValue{Raw: vals[0], Values: vals, RawBytes: raw}
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
//
// For fewer round-trips against capable devices, prefer ReadPropertiesMultiple,
// which uses a single ReadPropertyMultiple request and falls back to this
// per-property path automatically.
func (c *Client) ReadProperties(ctx context.Context, target Target, obj Object, pids []types.PropertyIdentifier) ([]PropertyResult, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	return c.readPropertiesIndividually(ctx, dst, obj, pids), nil
}

// ReadSpec requests several properties of one object for ReadMultiple.
type ReadSpec struct {
	// Object is the object to read from.
	Object Object
	// Properties are the property identifiers to read. Array properties are read
	// whole; use ReadProperty with AtIndex for a single element.
	Properties []types.PropertyIdentifier
}

// ObjectResult pairs an object with the per-property results of a multi-read.
type ObjectResult struct {
	Object     Object
	Properties []PropertyResult
}

// ReadPropertiesMultiple reads several properties of one object using a single
// ReadPropertyMultiple (RPM) request — one round-trip instead of one per
// property. Results are returned in the same order as pids, with per-property
// errors captured in each PropertyResult rather than aborting.
//
// If the device does not support RPM, the response would not fit a single APDU
// (this client does not reassemble segmented responses), or the device rejects
// the whole request because a requested property or object is not applicable,
// the call transparently falls back to reading each property individually. The
// result shape is the same either way, and the fallback confines any such error
// to the offending property so the rest still return values. Any other failure
// (e.g. a timeout) is returned as the error.
func (c *Client) ReadPropertiesMultiple(ctx context.Context, target Target, obj Object, pids []types.PropertyIdentifier) ([]PropertyResult, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, nil
	}

	results, err := c.readPropertyMultiple(ctx, dst, []ReadSpec{{Object: obj, Properties: pids}})
	if err != nil {
		if rpmUnusable(err) {
			return c.readPropertiesIndividually(ctx, dst, obj, pids), nil
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("read-property-multiple returned no results for %s", obj)
	}
	return results[0].Properties, nil
}

// ReadMultiple reads several properties of several objects using a single
// ReadPropertyMultiple (RPM) request. It is the most efficient way to sample
// many points at once. Results are returned one ObjectResult per spec, in the
// same order as specs, each carrying its properties in the requested order.
// Per-property errors are captured in each PropertyResult.
//
// As with ReadPropertiesMultiple, if the device does not support RPM, the
// response would not fit a single APDU, or it rejects the whole request over an
// inapplicable property or object, the call falls back to reading each property
// individually.
func (c *Client) ReadMultiple(ctx context.Context, target Target, specs []ReadSpec) ([]ObjectResult, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, nil
	}

	results, err := c.readPropertyMultiple(ctx, dst, specs)
	if err != nil {
		if rpmUnusable(err) {
			return c.readMultipleIndividually(ctx, dst, specs), nil
		}
		return nil, err
	}
	return results, nil
}

// readPropertyMultiple performs one RPM request for specs against an
// already-resolved address and maps the ACK back to the requested order.
func (c *Client) readPropertyMultiple(ctx context.Context, dst netprim.Address, specs []ReadSpec) ([]ObjectResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.requestBudget())
	defer cancel()

	accessSpecs := make([]apdu.ReadAccessSpecification, 0, len(specs))
	for _, spec := range specs {
		props := make([]apdu.PropertyReference, 0, len(spec.Properties))
		for _, pid := range spec.Properties {
			props = append(props, apdu.PropertyReference{PropertyIdentifier: pid})
		}
		accessSpecs = append(accessSpecs, apdu.ReadAccessSpecification{
			ObjectIdentifier: spec.Object.OID(),
			Properties:       props,
		})
	}

	req, err := apdu.NewReadPropertyMultipleRequest(accessSpecs)
	if err != nil {
		return nil, err
	}
	ack, err := c.apduClient().ReadPropertyMultiple(reqCtx, dst, req)
	if err != nil {
		return nil, err
	}
	return mapRPMResults(specs, ack), nil
}

// mapRPMResults turns an RPM ACK into ObjectResults ordered to match specs. It
// matches each requested (object, property) to a returned result; unmatched
// requests are reported as errors so the output always mirrors the request.
func mapRPMResults(specs []ReadSpec, ack apdu.ReadPropertyMultipleACK) []ObjectResult {
	// Index returned property results by (object, property) for order-independent
	// lookup, since some devices reorder results.
	type key struct {
		oid types.ObjectIdentifier
		pid types.PropertyIdentifier
	}
	index := make(map[key]apdu.ReadPropertyResult)
	for _, ar := range ack.Results {
		for _, pr := range ar.Results {
			index[key{ar.ObjectIdentifier, pr.PropertyIdentifier}] = pr
		}
	}

	out := make([]ObjectResult, 0, len(specs))
	for _, spec := range specs {
		oid := spec.Object.OID()
		props := make([]PropertyResult, 0, len(spec.Properties))
		for _, pid := range spec.Properties {
			pr, ok := index[key{oid, pid}]
			if !ok {
				props = append(props, PropertyResult{
					Property: pid,
					Err:      fmt.Errorf("no result returned for %s %s", spec.Object, PropertyName(pid)),
				})
				continue
			}
			props = append(props, propertyResultFromRPM(pid, pr))
		}
		out = append(out, ObjectResult{Object: spec.Object, Properties: props})
	}
	return out
}

// propertyResultFromRPM converts one RPM property entry into a PropertyResult,
// decoding either the value or the per-property error.
func propertyResultFromRPM(pid types.PropertyIdentifier, pr apdu.ReadPropertyResult) PropertyResult {
	if pr.Error != nil {
		class, code, derr := pr.DecodeError()
		if derr != nil {
			return PropertyResult{Property: pid, Err: derr}
		}
		return PropertyResult{
			Property: pid,
			Err: apdu.RemoteErrorAPDU{
				ServiceChoice: apdu.ServiceChoiceReadPropertyMultiple,
				ErrorClass:    class,
				ErrorCode:     code,
			},
		}
	}
	return PropertyResult{Property: pid, Value: decodeValue(pr.PropertyValue)}
}

// readPropertiesIndividually reads each property with its own ReadProperty
// request against an already-resolved address, capturing per-property errors.
// It is the per-property fallback for ReadPropertiesMultiple and the engine
// behind ReadProperties.
func (c *Client) readPropertiesIndividually(ctx context.Context, dst netprim.Address, obj Object, pids []types.PropertyIdentifier) []PropertyResult {
	results := make([]PropertyResult, 0, len(pids))
	for _, pid := range pids {
		v, rerr := c.readProperty(ctx, dst, obj.OID(), pid, nil)
		results = append(results, PropertyResult{Property: pid, Value: v, Err: rerr})
		c.pace(ctx)
	}
	return results
}

// readMultipleIndividually is the per-property fallback for ReadMultiple.
func (c *Client) readMultipleIndividually(ctx context.Context, dst netprim.Address, specs []ReadSpec) []ObjectResult {
	out := make([]ObjectResult, 0, len(specs))
	for _, spec := range specs {
		out = append(out, ObjectResult{
			Object:     spec.Object,
			Properties: c.readPropertiesIndividually(ctx, dst, spec.Object, spec.Properties),
		})
	}
	return out
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

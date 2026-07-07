package client

import (
	"context"
	"fmt"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/types"
	"github.com/worldiety/bacnet/encoding"
)

// WriteOptions configures a WriteProperty call.
type WriteOptions struct {
	// Priority is the BACnet write priority (1..16) for commandable properties.
	// Nil writes without a priority.
	Priority *uint8
	// ArrayIndex writes a single array element. Nil writes the whole property.
	ArrayIndex *uint32
}

// WriteOption mutates WriteOptions.
type WriteOption func(*WriteOptions)

// WithPriority sets the BACnet write priority (must be 1..16).
func WithPriority(p uint8) WriteOption {
	return func(o *WriteOptions) { o.Priority = &p }
}

// WriteAtIndex writes a single array element at the given index.
func WriteAtIndex(i uint32) WriteOption {
	return func(o *WriteOptions) { o.ArrayIndex = &i }
}

// WriteProperty writes a single property on an object of the target device.
// For a device-ID target the address is resolved first via Who-Is.
//
// Writing mutates a live device; callers are responsible for any confirmation
// workflow. Use WriteAndReadBack to verify the effect.
func (c *Client) WriteProperty(ctx context.Context, target Target, obj Object, pid types.PropertyIdentifier, value encoding.ApplicationValue, opts ...WriteOption) error {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return err
	}

	var o WriteOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.Priority != nil && (*o.Priority < 1 || *o.Priority > 16) {
		return fmt.Errorf("priority must be between 1 and 16")
	}

	encoded, err := encoding.EncodeApplicationValue(value)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}

	req, err := apdu.NewWritePropertyRequest(obj.OID(), pid, o.ArrayIndex, encoded, o.Priority)
	if err != nil {
		return fmt.Errorf("build write request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.requestBudget())
	defer cancel()

	if err := c.apduClient().WriteProperty(reqCtx, dst, req); err != nil {
		return err
	}
	return nil
}

// WriteAndReadBack writes a property and then reads it back, returning the
// value the device reports afterwards. The target is resolved once and reused
// for both operations.
func (c *Client) WriteAndReadBack(ctx context.Context, target Target, obj Object, pid types.PropertyIdentifier, value encoding.ApplicationValue, opts ...WriteOption) (PropertyValue, error) {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return PropertyValue{}, err
	}
	// Address the resolved device directly for both operations, preserving any
	// routing (remote network + MAC) so a routed device stays reachable.
	addrTarget := targetForAddress(dst)

	if err := c.WriteProperty(ctx, addrTarget, obj, pid, value, opts...); err != nil {
		return PropertyValue{}, err
	}

	var ro []ReadOption
	var wo WriteOptions
	for _, opt := range opts {
		opt(&wo)
	}
	if wo.ArrayIndex != nil {
		ro = append(ro, AtIndex(*wo.ArrayIndex))
	}
	return c.ReadProperty(ctx, addrTarget, obj, pid, ro...)
}

// PropertyWrite describes one property write within a WriteSpec.
type PropertyWrite struct {
	// Property is the property to write.
	Property types.PropertyIdentifier
	// Value is the value to write.
	Value encoding.ApplicationValue
	// ArrayIndex writes a single array element. Nil writes the whole property.
	ArrayIndex *uint32
	// Priority is the BACnet write priority (1..16) for commandable properties.
	// Nil writes without a priority.
	Priority *uint8
}

// WriteSpec groups several property writes for a single object.
type WriteSpec struct {
	Object Object
	Values []PropertyWrite
}

// WritePropertiesMultiple writes several properties (across one or more objects)
// in a single WritePropertyMultiple (WPM) request. This is both faster than
// issuing one WriteProperty per value and, per the BACnet standard, applied by
// the device as a single ordered operation.
//
// Unlike the read helpers, WritePropertiesMultiple does not fall back to
// individual writes when a device lacks WPM support: splitting an atomic
// multi-write into separate requests would silently change its semantics. If
// the device does not support WPM the underlying error is returned unchanged
// (see Describe); the caller can then choose to issue individual WriteProperty
// calls if per-write semantics are acceptable.
func (c *Client) WritePropertiesMultiple(ctx context.Context, target Target, writes []WriteSpec) error {
	dst, _, err := c.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	if len(writes) == 0 {
		return fmt.Errorf("no writes provided")
	}

	specs := make([]apdu.WriteAccessSpecification, 0, len(writes))
	for _, w := range writes {
		if len(w.Values) == 0 {
			return fmt.Errorf("write for %s has no values", w.Object)
		}
		values := make([]apdu.PropertyValueWrite, 0, len(w.Values))
		for _, pw := range w.Values {
			if pw.Priority != nil && (*pw.Priority < 1 || *pw.Priority > 16) {
				return fmt.Errorf("priority must be between 1 and 16")
			}
			encoded, err := encoding.EncodeApplicationValue(pw.Value)
			if err != nil {
				return fmt.Errorf("encode value for %s %s: %w", w.Object, PropertyName(pw.Property), err)
			}
			values = append(values, apdu.PropertyValueWrite{
				PropertyIdentifier: pw.Property,
				ArrayIndex:         pw.ArrayIndex,
				PropertyValue:      encoded,
				Priority:           pw.Priority,
			})
		}
		specs = append(specs, apdu.WriteAccessSpecification{
			ObjectIdentifier: w.Object.OID(),
			Values:           values,
		})
	}

	req, err := apdu.NewWritePropertyMultipleRequest(specs)
	if err != nil {
		return fmt.Errorf("build write-property-multiple request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.requestBudget())
	defer cancel()

	return c.apduClient().WritePropertyMultiple(reqCtx, dst, req)
}

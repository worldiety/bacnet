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
	// Address the resolved device directly for both operations.
	addrTarget := TargetAddr(dst.AddrPort)

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

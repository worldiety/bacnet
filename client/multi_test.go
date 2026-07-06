package client

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
	"github.com/worldiety/bacnet/encoding"
)

// fakeAPDU is a stub apdu.Client. It embeds the interface so only the methods
// used by the tests need be implemented; any other call panics.
type fakeAPDU struct {
	apdu.Client

	rpmACK  apdu.ReadPropertyMultipleACK
	rpmErr  error
	rpmReqs []apdu.ReadPropertyMultipleRequest

	// rpErr and rpValue drive the per-property ReadProperty fallback.
	rpValue []byte
	rpErr   error
	rpReqs  []apdu.ReadPropertyRequest

	wpmErr  error
	wpmReqs []apdu.WritePropertyMultipleRequest
}

func (f *fakeAPDU) ReadPropertyMultiple(_ context.Context, _ netprim.Address, req apdu.ReadPropertyMultipleRequest) (apdu.ReadPropertyMultipleACK, error) {
	f.rpmReqs = append(f.rpmReqs, req)
	if f.rpmErr != nil {
		return apdu.ReadPropertyMultipleACK{}, f.rpmErr
	}
	return f.rpmACK, nil
}

func (f *fakeAPDU) ReadProperty(_ context.Context, _ netprim.Address, req apdu.ReadPropertyRequest) (apdu.ReadPropertyACK, error) {
	f.rpReqs = append(f.rpReqs, req)
	if f.rpErr != nil {
		return apdu.ReadPropertyACK{}, f.rpErr
	}
	return apdu.ReadPropertyACK{PropertyValue: f.rpValue}, nil
}

func (f *fakeAPDU) WritePropertyMultiple(_ context.Context, _ netprim.Address, req apdu.WritePropertyMultipleRequest) error {
	f.wpmReqs = append(f.wpmReqs, req)
	return f.wpmErr
}

func fakeClient(f apdu.Client) *Client {
	return &Client{apduClientOverride: f}
}

// netip4 returns a valid IPv4 AddrPort for an address target so resolveTarget
// returns immediately without needing a live runtime.
func netip4(t *testing.T) netip.AddrPort {
	t.Helper()
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{10, 0, 0, 1}), 47808)
}

func appBytes(t *testing.T, v encoding.ApplicationValue) []byte {
	t.Helper()
	b, err := encoding.EncodeApplicationValue(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}

func errorBody(t *testing.T, class apdu.ErrorClass, code apdu.ErrorCode) []byte {
	t.Helper()
	cb := appBytes(t, encoding.AppEnum(uint32(class)))
	db := appBytes(t, encoding.AppEnum(uint32(code)))
	return append(cb, db...)
}

func TestReadPropertiesMultipleSuccess(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogInput, Instance: 7}
	oid := obj.OID()

	f := &fakeAPDU{
		rpmACK: apdu.ReadPropertyMultipleACK{
			Results: []apdu.ReadAccessResult{{
				ObjectIdentifier: oid,
				// Return results out of requested order to exercise re-matching.
				Results: []apdu.ReadPropertyResult{
					{
						PropertyIdentifier: types.PropertyIdentifierUnits,
						PropertyValue:      appBytes(t, encoding.AppEnum(62)),
					},
					{
						PropertyIdentifier: types.PropertyIdentifierPresentValue,
						PropertyValue:      appBytes(t, encoding.AppReal(21.5)),
					},
				},
			}},
		},
	}
	c := fakeClient(f)

	pids := []types.PropertyIdentifier{
		types.PropertyIdentifierPresentValue,
		types.PropertyIdentifierUnits,
	}
	results, err := c.ReadPropertiesMultiple(context.Background(), TargetAddr(netip4(t)), obj, pids)
	if err != nil {
		t.Fatalf("ReadPropertiesMultiple: %v", err)
	}
	if len(f.rpmReqs) != 1 {
		t.Fatalf("expected 1 RPM request, got %d", len(f.rpmReqs))
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	// Order must match the request, not the ACK.
	if results[0].Property != types.PropertyIdentifierPresentValue {
		t.Fatalf("results[0].Property = %v, want present-value", results[0].Property)
	}
	if f, ok := results[0].Value.Float64(); !ok || f != 21.5 {
		t.Fatalf("results[0] value = %v, %v; want 21.5", f, ok)
	}
	if results[1].Property != types.PropertyIdentifierUnits {
		t.Fatalf("results[1].Property = %v, want units", results[1].Property)
	}
	if disp := results[1].Value.Display(types.PropertyIdentifierUnits); disp != "62 (degrees-celsius)" {
		t.Fatalf("results[1] display = %q", disp)
	}
}

func TestReadPropertiesMultiplePerPropertyError(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogInput, Instance: 7}
	oid := obj.OID()

	f := &fakeAPDU{
		rpmACK: apdu.ReadPropertyMultipleACK{
			Results: []apdu.ReadAccessResult{{
				ObjectIdentifier: oid,
				Results: []apdu.ReadPropertyResult{
					{
						PropertyIdentifier: types.PropertyIdentifierPresentValue,
						PropertyValue:      appBytes(t, encoding.AppReal(21.5)),
					},
					{
						PropertyIdentifier: types.PropertyIdentifierUnits,
						Error:              errorBody(t, apdu.ErrorClassProperty, apdu.ErrorCodePropertyUnknownProperty),
					},
				},
			}},
		},
	}
	c := fakeClient(f)

	pids := []types.PropertyIdentifier{
		types.PropertyIdentifierPresentValue,
		types.PropertyIdentifierUnits,
	}
	results, err := c.ReadPropertiesMultiple(context.Background(), TargetAddr(netip4(t)), obj, pids)
	if err != nil {
		t.Fatalf("ReadPropertiesMultiple: %v", err)
	}
	if results[0].Err != nil {
		t.Fatalf("results[0].Err = %v, want nil", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatal("results[1].Err = nil, want a remote error")
	}
	if !IsRemoteError(results[1].Err) {
		t.Fatalf("results[1].Err should be a remote error, got %v", results[1].Err)
	}
	var re apdu.RemoteErrorAPDU
	if !errors.As(results[1].Err, &re) || re.ErrorCode != apdu.ErrorCodePropertyUnknownProperty {
		t.Fatalf("results[1].Err = %v, want unknown-property", results[1].Err)
	}
}

func TestReadPropertiesMultipleFallback(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogInput, Instance: 7}

	// Each of these RPM errors must trigger the per-property fallback.
	unusable := []error{
		apdu.RemoteRejectAPDU{RejectReason: apdu.RejectReasonUnrecognizedService},
		apdu.RemoteErrorAPDU{ErrorClass: apdu.ErrorClassServices, ErrorCode: apdu.ErrorCodeServicesServiceRequestDenied},
		// A whole-request Error-PDU for a property/object the object does not
		// have (e.g. a network-port property inapplicable to the port).
		apdu.RemoteErrorAPDU{ErrorClass: apdu.ErrorClassProperty, ErrorCode: apdu.ErrorCodePropertyUnknownProperty},
		apdu.RemoteErrorAPDU{ErrorClass: apdu.ErrorClassObject, ErrorCode: apdu.ErrorCodeObjectUnknownObject},
		apdu.RemoteErrorAPDU{ErrorClass: apdu.ErrorClassObject, ErrorCode: apdu.ErrorCodeObjectUnsupportedObjectType},
		apdu.RemoteErrorAPDU{ErrorClass: apdu.ErrorClassProperty, ErrorCode: apdu.ErrorCodePropertyInvalidArrayIndex},
		apdu.RemoteAbortAPDU{AbortReason: apdu.AbortReasonSegmentationNotSupported},
		apdu.RemoteAbortAPDU{AbortReason: apdu.AbortReasonBufferOverflow},
		apdu.RemoteAbortAPDU{AbortReason: apdu.AbortReasonAPDUTooLong},
		apdu.ErrSegmentationNotSupported,
	}

	for _, ue := range unusable {
		f := &fakeAPDU{
			rpmErr:  ue,
			rpValue: appBytes(t, encoding.AppReal(21.5)),
		}
		c := fakeClient(f)

		pids := []types.PropertyIdentifier{types.PropertyIdentifierPresentValue}
		results, err := c.ReadPropertiesMultiple(context.Background(), TargetAddr(netip4(t)), obj, pids)
		if err != nil {
			t.Fatalf("[%v] ReadPropertiesMultiple: %v", ue, err)
		}
		if len(f.rpReqs) != 1 {
			t.Fatalf("[%v] expected 1 per-property ReadProperty fallback, got %d", ue, len(f.rpReqs))
		}
		if len(results) != 1 || results[0].Err != nil {
			t.Fatalf("[%v] fallback results = %#v", ue, results)
		}
		if fv, ok := results[0].Value.Float64(); !ok || fv != 21.5 {
			t.Fatalf("[%v] fallback value = %v, %v", ue, fv, ok)
		}
	}
}

// fakeFallbackAPDU serves distinct per-property values so a fallback read can
// return different results for different properties.
type fakeFallbackAPDU struct {
	apdu.Client
	values map[types.PropertyIdentifier][]byte
	errs   map[types.PropertyIdentifier]error
}

func (f *fakeFallbackAPDU) ReadPropertyMultiple(_ context.Context, _ netprim.Address, _ apdu.ReadPropertyMultipleRequest) (apdu.ReadPropertyMultipleACK, error) {
	// Simulate a device that rejects the whole RPM because one requested
	// property is unknown to the object (the reported real-world failure).
	return apdu.ReadPropertyMultipleACK{}, apdu.RemoteErrorAPDU{
		ServiceChoice: apdu.ServiceChoiceReadPropertyMultiple,
		ErrorClass:    apdu.ErrorClassProperty,
		ErrorCode:     apdu.ErrorCodePropertyUnknownProperty,
	}
}

func (f *fakeFallbackAPDU) ReadProperty(_ context.Context, _ netprim.Address, req apdu.ReadPropertyRequest) (apdu.ReadPropertyACK, error) {
	if err, ok := f.errs[req.PropertyIdentifier]; ok {
		return apdu.ReadPropertyACK{}, err
	}
	return apdu.ReadPropertyACK{PropertyValue: f.values[req.PropertyIdentifier]}, nil
}

// TestReadPropertiesMultipleWholeRequestUnknownProperty reproduces the reported
// failure: a device rejects the entire RPM with property/unknown-property. The
// fallback must read each property individually so the valid ones return values
// and only the unknown one carries a per-property error.
func TestReadPropertiesMultipleWholeRequestUnknownProperty(t *testing.T) {
	// network-port (object type 56) is not a named constant but is a valid type;
	// use the raw value to mirror the reported real-world object.
	obj := Object{Type: types.ObjectType(56), Instance: 1}
	good := types.PropertyIdentifierObjectName
	bad := types.PropertyIdentifierPresentValue // not applicable to network-port

	f := &fakeFallbackAPDU{
		values: map[types.PropertyIdentifier][]byte{
			good: appBytes(t, encoding.AppCharacterString("Port 1")),
		},
		errs: map[types.PropertyIdentifier]error{
			bad: apdu.RemoteErrorAPDU{
				ServiceChoice: apdu.ServiceChoiceReadProperty,
				ErrorClass:    apdu.ErrorClassProperty,
				ErrorCode:     apdu.ErrorCodePropertyUnknownProperty,
			},
		},
	}
	c := fakeClient(f)

	results, err := c.ReadPropertiesMultiple(context.Background(), TargetAddr(netip4(t)), obj,
		[]types.PropertyIdentifier{good, bad})
	if err != nil {
		t.Fatalf("ReadPropertiesMultiple should fall back, not fail: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	// Good property returns its value.
	if results[0].Property != good || results[0].Err != nil {
		t.Fatalf("results[0] = %#v, want %s with no error", results[0], PropertyName(good))
	}
	if s, ok := results[0].Value.Text(); !ok || s != "Port 1" {
		t.Fatalf("results[0] text = %q, %v", s, ok)
	}
	// Bad property is isolated to a per-property remote error.
	if results[1].Property != bad || results[1].Err == nil {
		t.Fatalf("results[1] = %#v, want %s with an error", results[1], PropertyName(bad))
	}
	if !IsRemoteError(results[1].Err) {
		t.Fatalf("results[1].Err should be a remote error, got %v", results[1].Err)
	}
}

func TestReadPropertiesMultipleNonUnusableErrorPropagates(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogInput, Instance: 7}
	f := &fakeAPDU{rpmErr: apdu.ErrAPDUTimeout}
	c := fakeClient(f)

	_, err := c.ReadPropertiesMultiple(context.Background(), TargetAddr(netip4(t)), obj,
		[]types.PropertyIdentifier{types.PropertyIdentifierPresentValue})
	if err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
	if len(f.rpReqs) != 0 {
		t.Fatalf("timeout must not trigger fallback; got %d ReadProperty calls", len(f.rpReqs))
	}
}

func TestReadMultipleSuccess(t *testing.T) {
	obj1 := Object{Type: types.ObjectTypeAnalogInput, Instance: 1}
	obj2 := Object{Type: types.ObjectTypeAnalogValue, Instance: 2}

	f := &fakeAPDU{
		rpmACK: apdu.ReadPropertyMultipleACK{
			Results: []apdu.ReadAccessResult{
				{
					ObjectIdentifier: obj1.OID(),
					Results: []apdu.ReadPropertyResult{{
						PropertyIdentifier: types.PropertyIdentifierPresentValue,
						PropertyValue:      appBytes(t, encoding.AppReal(1.5)),
					}},
				},
				{
					ObjectIdentifier: obj2.OID(),
					Results: []apdu.ReadPropertyResult{{
						PropertyIdentifier: types.PropertyIdentifierPresentValue,
						PropertyValue:      appBytes(t, encoding.AppReal(2.5)),
					}},
				},
			},
		},
	}
	c := fakeClient(f)

	specs := []ReadSpec{
		{Object: obj1, Properties: []types.PropertyIdentifier{types.PropertyIdentifierPresentValue}},
		{Object: obj2, Properties: []types.PropertyIdentifier{types.PropertyIdentifierPresentValue}},
	}
	results, err := c.ReadMultiple(context.Background(), TargetAddr(netip4(t)), specs)
	if err != nil {
		t.Fatalf("ReadMultiple: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	if results[0].Object != obj1 || results[1].Object != obj2 {
		t.Fatalf("object order mismatch: %v, %v", results[0].Object, results[1].Object)
	}
	if v, ok := results[0].Properties[0].Value.Float64(); !ok || v != 1.5 {
		t.Fatalf("obj1 value = %v, %v", v, ok)
	}
	if v, ok := results[1].Properties[0].Value.Float64(); !ok || v != 2.5 {
		t.Fatalf("obj2 value = %v, %v", v, ok)
	}
}

func TestReadPropertiesMultipleMissingResult(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogInput, Instance: 7}
	// ACK omits the requested property entirely.
	f := &fakeAPDU{
		rpmACK: apdu.ReadPropertyMultipleACK{
			Results: []apdu.ReadAccessResult{{ObjectIdentifier: obj.OID()}},
		},
	}
	c := fakeClient(f)

	results, err := c.ReadPropertiesMultiple(context.Background(), TargetAddr(netip4(t)), obj,
		[]types.PropertyIdentifier{types.PropertyIdentifierPresentValue})
	if err != nil {
		t.Fatalf("ReadPropertiesMultiple: %v", err)
	}
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("missing result should surface an error: %#v", results)
	}
}

func TestWritePropertiesMultiple(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogValue, Instance: 1}
	f := &fakeAPDU{}
	c := fakeClient(f)

	writes := []WriteSpec{{
		Object: obj,
		Values: []PropertyWrite{
			{Property: types.PropertyIdentifierPresentValue, Value: ValueReal(21.5)},
		},
	}}
	if err := c.WritePropertiesMultiple(context.Background(), TargetAddr(netip4(t)), writes); err != nil {
		t.Fatalf("WritePropertiesMultiple: %v", err)
	}
	if len(f.wpmReqs) != 1 {
		t.Fatalf("expected 1 WPM request, got %d", len(f.wpmReqs))
	}
	got := f.wpmReqs[0]
	if len(got.Writes) != 1 || got.Writes[0].ObjectIdentifier != obj.OID() {
		t.Fatalf("WPM request object mismatch: %#v", got.Writes)
	}
	if len(got.Writes[0].Values) != 1 || got.Writes[0].Values[0].PropertyIdentifier != types.PropertyIdentifierPresentValue {
		t.Fatalf("WPM request value mismatch: %#v", got.Writes[0].Values)
	}
}

func TestWritePropertiesMultipleNoFallback(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogValue, Instance: 1}
	// Even an "unsupported" signal must propagate for writes (no fallback).
	f := &fakeAPDU{wpmErr: apdu.RemoteRejectAPDU{RejectReason: apdu.RejectReasonUnrecognizedService}}
	c := fakeClient(f)

	err := c.WritePropertiesMultiple(context.Background(), TargetAddr(netip4(t)), []WriteSpec{{
		Object: obj,
		Values: []PropertyWrite{{Property: types.PropertyIdentifierPresentValue, Value: ValueReal(1)}},
	}})
	if err == nil {
		t.Fatal("expected WPM error to propagate, got nil")
	}
}

func TestWritePropertiesMultipleValidation(t *testing.T) {
	obj := Object{Type: types.ObjectTypeAnalogValue, Instance: 1}
	c := fakeClient(&fakeAPDU{})

	// Empty writes.
	if err := c.WritePropertiesMultiple(context.Background(), TargetAddr(netip4(t)), nil); err == nil {
		t.Fatal("empty writes should error")
	}
	// Bad priority.
	bad := uint8(0)
	err := c.WritePropertiesMultiple(context.Background(), TargetAddr(netip4(t)), []WriteSpec{{
		Object: obj,
		Values: []PropertyWrite{{Property: types.PropertyIdentifierPresentValue, Value: ValueReal(1), Priority: &bad}},
	}})
	if err == nil {
		t.Fatal("priority 0 should error")
	}
}

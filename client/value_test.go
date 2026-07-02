package client

import (
	"testing"

	"github.com/worldiety/bacnet/common/types"
	"github.com/worldiety/bacnet/encoding"
)

func TestParseValueRoundTrip(t *testing.T) {
	tests := []struct {
		in       string
		wantDisp string
	}{
		{"real:21.5", "21.5"},
		{"unsigned:3", "3"},
		{"int:-5", "-5"},
		{"double:1.25", "1.25"},
		{"bool:true", "true"},
		{"enum:2", "enum(2)"},
		{"string:Küche", `"Küche"`},
		{"null:", "null"},
		{"oid:analog-value:1", "analog-value:1"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			av, err := ParseValue(tt.in)
			if err != nil {
				t.Fatalf("ParseValue(%q): %v", tt.in, err)
			}
			enc, err := encoding.EncodeApplicationValue(av)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			pv := decodeValue(enc)
			if !pv.Decoded() {
				t.Fatalf("value did not decode: raw=%x", pv.RawBytes)
			}
			if got := pv.Display(0); got != tt.wantDisp {
				t.Fatalf("Display = %q, want %q", got, tt.wantDisp)
			}
		})
	}
}

func TestParseValueErrors(t *testing.T) {
	for _, in := range []string{"real:notafloat", "bogus:1", "noseparator", "unsigned:-1"} {
		if _, err := ParseValue(in); err == nil {
			t.Fatalf("ParseValue(%q) should error", in)
		}
	}
}

func TestPropertyValueAccessors(t *testing.T) {
	pv := decodeValue(mustEncode(t, ValueReal(21.5)))
	if f, ok := pv.Float64(); !ok || f != 21.5 {
		t.Fatalf("Float64 = %v, %v", f, ok)
	}

	pv = decodeValue(mustEncode(t, ValueUnsigned(7)))
	if u, ok := pv.Uint(); !ok || u != 7 {
		t.Fatalf("Uint = %v, %v", u, ok)
	}

	pv = decodeValue(mustEncode(t, ValueBool(true)))
	if b, ok := pv.Bool(); !ok || !b {
		t.Fatalf("Bool = %v, %v", b, ok)
	}

	pv = decodeValue(mustEncode(t, ValueString("hello")))
	if s, ok := pv.Text(); !ok || s != "hello" {
		t.Fatalf("Text = %q, %v", s, ok)
	}

	pv = decodeValue(mustEncode(t, ValueNull()))
	if !pv.IsNull() {
		t.Fatal("IsNull = false, want true")
	}
}

func TestDisplayWithContext(t *testing.T) {
	// units enum should render the unit name.
	pv := decodeValue(mustEncode(t, ValueUnsigned(62)))
	if got := pv.Display(types.PropertyIdentifierUnits); got != "62 (degrees-celsius)" {
		t.Fatalf("units display = %q", got)
	}
	// object-type enum should render the type name.
	pv = decodeValue(mustEncode(t, ValueUnsigned(8)))
	if got := pv.Display(types.PropertyIdentifierObjectType); got != "8 (device)" {
		t.Fatalf("object-type display = %q", got)
	}
}

func TestRecoverCharacterStringLatin1(t *testing.T) {
	// tag 7 (char string), value length 3 (LVT nibble): charset 0x00 + 0xFC
	// ('ü' in Latin-1, invalid UTF-8) + 'x'. Layout matches the library's own
	// encoding of a 2-char charset-0 string ("73 00 <b0> <b1>").
	raw := []byte{0x73, 0x00, 0xFC, 0x78}
	pv := decodeValue(raw)
	// The library rejects invalid UTF-8; our recovery interprets it as Latin-1.
	if got := pv.Display(0); got != "üx" {
		t.Fatalf("recovered display = %q, want üx", got)
	}
}

func mustEncode(t *testing.T, v encoding.ApplicationValue) []byte {
	t.Helper()
	b, err := encoding.EncodeApplicationValue(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}

func TestNameLookups(t *testing.T) {
	if got := ObjectTypeName(types.ObjectTypeDevice); got != "device" {
		t.Fatalf("ObjectTypeName(device) = %q", got)
	}
	if ot, ok := ObjectTypeByName("analog-value"); !ok || ot != types.ObjectTypeAnalogValue {
		t.Fatalf("ObjectTypeByName(analog-value) = %d, %v", ot, ok)
	}
	if got := PropertyName(types.PropertyIdentifierPresentValue); got != "present-value" {
		t.Fatalf("PropertyName(present-value) = %q", got)
	}
	if got := PropertyName(139); got != "protocol-revision" {
		t.Fatalf("PropertyName(139) = %q, want protocol-revision", got)
	}
	if pid, ok := PropertyByName("object-list"); !ok || pid != 76 {
		t.Fatalf("PropertyByName(object-list) = %d, %v", pid, ok)
	}
	if got := UnitsName(62); got != "degrees-celsius" {
		t.Fatalf("UnitsName(62) = %q", got)
	}
	if got := UnitsName(99999); got != "99999" {
		t.Fatalf("UnitsName(unknown) = %q, want numeric", got)
	}
}

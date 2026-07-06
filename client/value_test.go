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

func TestCharacterStringLatin1InCharset0(t *testing.T) {
	// tag 7 (char string), value length 3 (LVT nibble): charset 0x00 + 0xFC
	// ('ü' in Latin-1, invalid UTF-8) + 'x'. Layout matches the library's own
	// encoding of a 2-char charset-0 string ("73 00 <b0> <b1>").
	raw := []byte{0x73, 0x00, 0xFC, 0x78}
	pv := decodeValue(raw)
	// The library now recovers Latin-1-in-charset-0 into a proper decoded value.
	if !pv.Decoded() {
		t.Fatalf("value should now decode; raw=%x", pv.RawBytes)
	}
	if s, ok := pv.Text(); !ok || s != "üx" {
		t.Fatalf("Text = %q, %v; want üx, true", s, ok)
	}
	// Display quotes character strings.
	if got := pv.Display(0); got != `"üx"` {
		t.Fatalf("Display = %q, want %q", got, `"üx"`)
	}
	if cs, ok := pv.Charset(); !ok || cs != encoding.CharacterSetUTF8 {
		t.Fatalf("Charset = %d, %v; want 0, true", cs, ok)
	}
}

func TestCharacterStringUCS2(t *testing.T) {
	// Kieback&Peter and other European controllers emit UCS-2 (UTF-16BE,
	// character set 4) for names such as "Außentemperatur".
	body := []byte{0x04} // charset 4
	for _, r := range "Außentemperatur" {
		body = append(body, byte(uint16(r)>>8), byte(uint16(r)))
	}
	raw := encoding.EncodeApplicationPrimitive(uint8(encoding.AppTagCharacterString), body)
	pv := decodeValue(raw)
	if !pv.Decoded() {
		t.Fatalf("UCS-2 value should decode; raw=%x", pv.RawBytes)
	}
	if s, ok := pv.Text(); !ok || s != "Außentemperatur" {
		t.Fatalf("Text = %q, %v; want Außentemperatur, true", s, ok)
	}
	if cs, ok := pv.Charset(); !ok || cs != encoding.CharacterSetUCS2 {
		t.Fatalf("Charset = %d, %v; want 4 (UCS-2), true", cs, ok)
	}
}

func TestCharsetNonCharacterString(t *testing.T) {
	// Charset must report false for non-character-string values.
	pv := decodeValue(mustEncode(t, ValueReal(21.5)))
	if _, ok := pv.Charset(); ok {
		t.Fatal("Charset should be false for a Real value")
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

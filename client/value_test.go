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

func TestMultiValueList(t *testing.T) {
	// object-list style body: three concatenated object identifiers.
	oids := []types.ObjectIdentifier{
		mustOID(t, types.ObjectTypeDevice, 1),
		mustOID(t, types.ObjectTypeAnalogInput, 2),
		mustOID(t, types.ObjectTypeAnalogValue, 3),
	}
	var body []byte
	for _, o := range oids {
		body = append(body, mustEncode(t, encoding.AppObjectIdentifier(o))...)
	}

	pv := decodeValue(body)
	if !pv.Decoded() {
		t.Fatalf("list should decode; raw=%x", pv.RawBytes)
	}
	if pv.Len() != 3 {
		t.Fatalf("Len = %d, want 3", pv.Len())
	}
	if !pv.IsList() {
		t.Fatal("IsList = false, want true")
	}
	// Raw is the first element for backward compatibility.
	if got, ok := pv.ObjectID(); !ok || got != oids[0] {
		t.Fatalf("ObjectID (first) = %v, %v; want %v", got, ok, oids[0])
	}
	got, ok := pv.ObjectIDs()
	if !ok {
		t.Fatal("ObjectIDs ok = false, want true")
	}
	if len(got) != 3 || got[0] != oids[0] || got[1] != oids[1] || got[2] != oids[2] {
		t.Fatalf("ObjectIDs = %v, want %v", got, oids)
	}
	want := "[device:1, analog-input:2, analog-value:3]"
	if disp := pv.Display(0); disp != want {
		t.Fatalf("Display = %q, want %q", disp, want)
	}
}

func TestScalarStillSingleValue(t *testing.T) {
	// A scalar must behave exactly as before: Len 1, not a list, bare Display.
	pv := decodeValue(mustEncode(t, ValueReal(21.5)))
	if pv.Len() != 1 {
		t.Fatalf("Len = %d, want 1", pv.Len())
	}
	if pv.IsList() {
		t.Fatal("IsList = true, want false for scalar")
	}
	if disp := pv.Display(0); disp != "21.5" {
		t.Fatalf("Display = %q, want 21.5", disp)
	}
	// ObjectIDs must fail cleanly for a non-OID value.
	if _, ok := pv.ObjectIDs(); ok {
		t.Fatal("ObjectIDs ok = true, want false for a Real")
	}
}

func TestUndecodableValue(t *testing.T) {
	// A context-tagged byte is not a plain application value: nothing decodes,
	// but the raw bytes are retained.
	pv := decodeValue([]byte{0x09, 0x01})
	if pv.Decoded() {
		t.Fatal("Decoded = true, want false")
	}
	if pv.Len() != 0 {
		t.Fatalf("Len = %d, want 0", pv.Len())
	}
	if len(pv.RawBytes) != 2 {
		t.Fatalf("RawBytes len = %d, want 2", len(pv.RawBytes))
	}
}

func mustOID(t *testing.T, ot types.ObjectType, inst uint32) types.ObjectIdentifier {
	t.Helper()
	oid, err := types.NewObjectIdentifier(ot, inst)
	if err != nil {
		t.Fatalf("NewObjectIdentifier: %v", err)
	}
	return oid
}

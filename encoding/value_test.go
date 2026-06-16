package encoding

import (
	"errors"
	"math"
	"testing"
)

// TestEncodeNull verifies that EncodeNull produces the correct single-byte tag.
func TestEncodeNull(t *testing.T) {
	got := EncodeNull()
	if len(got) != 1 || got[0] != 0x00 {
		t.Errorf("EncodeNull() = %v, want [0x00]", got)
	}
}

// TestEncodeDecodeBoolean verifies application-tag Boolean encode and DecodeBoolean roundtrip.
func TestEncodeDecodeBoolean(t *testing.T) {
	tests := []struct {
		name    string
		input   bool
		wantTag byte
	}{
		{name: "false", input: false, wantTag: 0x10},
		{name: "true", input: true, wantTag: 0x11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeBoolean(tt.input)
			if len(got) != 1 || got[0] != tt.wantTag {
				t.Errorf("EncodeBoolean(%v) = %v, want [0x%02X]", tt.input, got, tt.wantTag)
			}

			val, n, err := DecodeBoolean(got)
			if err != nil {
				t.Fatalf("DecodeBoolean: %v", err)
			}
			if n != 1 {
				t.Errorf("DecodeBoolean consumed %d bytes, want 1", n)
			}
			if val != tt.input {
				t.Errorf("DecodeBoolean = %v, want %v", val, tt.input)
			}
		})
	}
}

// TestDecodeBooleanErrors verifies DecodeBoolean rejects bad input.
func TestDecodeBooleanErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty", input: []byte{}},
		{name: "wrong tag", input: []byte{0x21}}, // unsigned tag, not boolean
		{name: "context tag", input: []byte{0x19}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DecodeBoolean(tt.input)
			if !errors.Is(err, ErrDecodeFailure) {
				t.Errorf("DecodeBoolean(%v): want ErrDecodeFailure, got %v", tt.input, err)
			}
		})
	}
}

// TestEncodeBooleanValue verifies the context-primitive value bytes.
func TestEncodeBooleanValue(t *testing.T) {
	if got := EncodeBooleanValue(false); len(got) != 1 || got[0] != 0x00 {
		t.Errorf("EncodeBooleanValue(false) = %v, want [0x00]", got)
	}
	if got := EncodeBooleanValue(true); len(got) != 1 || got[0] != 0x01 {
		t.Errorf("EncodeBooleanValue(true) = %v, want [0x01]", got)
	}
}

// TestEncodeDecodeSigned verifies round-trip for signed integers.
func TestEncodeDecodeSigned(t *testing.T) {
	tests := []struct {
		name  string
		input int32
		want  []byte
	}{
		{name: "zero", input: 0, want: []byte{0x00}},
		{name: "positive 1-byte", input: 127, want: []byte{0x7F}},
		{name: "negative 1-byte", input: -1, want: []byte{0xFF}},
		{name: "negative min 1-byte", input: -128, want: []byte{0x80}},
		{name: "positive 2-byte", input: 128, want: []byte{0x00, 0x80}},
		{name: "negative 2-byte", input: -129, want: []byte{0xFF, 0x7F}},
		{name: "positive 3-byte", input: 0x7FFFFF, want: []byte{0x7F, 0xFF, 0xFF}},
		{name: "negative 3-byte", input: -0x800000, want: []byte{0x80, 0x00, 0x00}},
		{name: "positive 4-byte", input: math.MaxInt32, want: []byte{0x7F, 0xFF, 0xFF, 0xFF}},
		{name: "negative 4-byte", input: math.MinInt32, want: []byte{0x80, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeSigned(tt.input)
			if !bytesEqual(got, tt.want) {
				t.Errorf("EncodeSigned(%d) = %v, want %v", tt.input, got, tt.want)
			}

			dec, err := DecodeSigned(got)
			if err != nil {
				t.Fatalf("DecodeSigned: %v", err)
			}
			if dec != tt.input {
				t.Errorf("DecodeSigned roundtrip: got %d, want %d", dec, tt.input)
			}
		})
	}
}

// TestDecodeSignedErrors verifies DecodeSigned rejects bad lengths.
func TestDecodeSignedErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty", input: []byte{}},
		{name: "too long", input: []byte{1, 2, 3, 4, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeSigned(tt.input)
			if !errors.Is(err, ErrDecodeFailure) {
				t.Errorf("DecodeSigned(%v): want ErrDecodeFailure, got %v", tt.input, err)
			}
		})
	}
}

// TestEncodeDecodeReal verifies IEEE 754 single-precision roundtrip.
func TestEncodeDecodeReal(t *testing.T) {
	tests := []struct {
		name  string
		input float32
	}{
		{name: "zero", input: 0},
		{name: "one", input: 1.0},
		{name: "negative", input: -3.14},
		{name: "large", input: 1e30},
		{name: "NaN", input: float32(math.NaN())},
		{name: "+Inf", input: float32(math.Inf(1))},
		{name: "-Inf", input: float32(math.Inf(-1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeReal(tt.input)
			if len(encoded) != 4 {
				t.Fatalf("EncodeReal produced %d bytes, want 4", len(encoded))
			}
			dec, err := DecodeReal(encoded)
			if err != nil {
				t.Fatalf("DecodeReal: %v", err)
			}
			if math.IsNaN(float64(tt.input)) {
				if !math.IsNaN(float64(dec)) {
					t.Errorf("DecodeReal: want NaN, got %v", dec)
				}
				return
			}
			if dec != tt.input {
				t.Errorf("DecodeReal: got %v, want %v", dec, tt.input)
			}
		})
	}
}

// TestDecodeRealError verifies DecodeReal rejects wrong lengths.
func TestDecodeRealError(t *testing.T) {
	_, err := DecodeReal([]byte{1, 2, 3})
	if !errors.Is(err, ErrDecodeFailure) {
		t.Errorf("want ErrDecodeFailure, got %v", err)
	}
}

// TestEncodeDecodeDouble verifies IEEE 754 double-precision roundtrip.
func TestEncodeDecodeDouble(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{name: "zero", input: 0},
		{name: "pi", input: math.Pi},
		{name: "negative", input: -2.718281828},
		{name: "+Inf", input: math.Inf(1)},
		{name: "NaN", input: math.NaN()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeDouble(tt.input)
			if len(encoded) != 8 {
				t.Fatalf("EncodeDouble produced %d bytes, want 8", len(encoded))
			}
			dec, err := DecodeDouble(encoded)
			if err != nil {
				t.Fatalf("DecodeDouble: %v", err)
			}
			if math.IsNaN(tt.input) {
				if !math.IsNaN(dec) {
					t.Errorf("DecodeDouble: want NaN, got %v", dec)
				}
				return
			}
			if dec != tt.input {
				t.Errorf("DecodeDouble: got %v, want %v", dec, tt.input)
			}
		})
	}
}

// TestDecodeDoubleError verifies DecodeDouble rejects wrong lengths.
func TestDecodeDoubleError(t *testing.T) {
	_, err := DecodeDouble([]byte{1, 2, 3, 4})
	if !errors.Is(err, ErrDecodeFailure) {
		t.Errorf("want ErrDecodeFailure, got %v", err)
	}
}

// TestEncodeDecodeOctetString verifies OctetString defensive copy.
func TestEncodeDecodeOctetString(t *testing.T) {
	original := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encoded := EncodeOctetStringValue(original)
	if !bytesEqual(encoded, original) {
		t.Errorf("EncodeOctetStringValue: got %v, want %v", encoded, original)
	}
	// Verify it's a copy.
	original[0] = 0x00
	if encoded[0] == 0x00 {
		t.Error("EncodeOctetStringValue did not copy the input slice")
	}

	decoded := DecodeOctetStringValue(encoded)
	if !bytesEqual(decoded, encoded) {
		t.Errorf("DecodeOctetStringValue: got %v, want %v", decoded, encoded)
	}
}

// TestEncodeDecodeEnumerated verifies Enumerated roundtrip (wire-identical to Unsigned).
func TestEncodeDecodeEnumerated(t *testing.T) {
	tests := []struct {
		name  string
		input uint32
	}{
		{name: "zero", input: 0},
		{name: "small", input: 5},
		{name: "large", input: 0x01020304},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := EncodeEnumeratedValue(tt.input)
			dec, err := DecodeEnumeratedValue(enc)
			if err != nil {
				t.Fatalf("DecodeEnumeratedValue: %v", err)
			}
			if dec != tt.input {
				t.Errorf("roundtrip: got %d, want %d", dec, tt.input)
			}
		})
	}
}

// TestEncodeDecodeBitString verifies BitString roundtrip including unused-bit handling.
func TestEncodeDecodeBitString(t *testing.T) {
	tests := []struct {
		name  string
		bits  []bool
		want  []byte
	}{
		{
			name:  "empty",
			bits:  []bool{},
			want:  []byte{0x00},
		},
		{
			name:  "3 bits firstItem+moreItems",
			bits:  []bool{true, false, true},
			want:  []byte{0x05, 0xA0}, // unused=5, data=10100000
		},
		{
			name:  "8 bits all set",
			bits:  []bool{true, true, true, true, true, true, true, true},
			want:  []byte{0x00, 0xFF},
		},
		{
			name:  "9 bits",
			bits:  []bool{true, false, false, false, false, false, false, false, true},
			want:  []byte{0x07, 0x80, 0x80}, // unused=7, data=10000000 10000000
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := NewBitString(tt.bits)
			encoded := EncodeBitStringValue(bs)
			if !bytesEqual(encoded, tt.want) {
				t.Errorf("EncodeBitStringValue: got %v, want %v", encoded, tt.want)
			}

			decoded, err := DecodeBitStringValue(encoded)
			if err != nil {
				t.Fatalf("DecodeBitStringValue: %v", err)
			}
			if len(decoded.Bits) != len(tt.bits) {
				t.Fatalf("decoded bit count %d, want %d", len(decoded.Bits), len(tt.bits))
			}
			for i, b := range tt.bits {
				if decoded.Bits[i] != b {
					t.Errorf("bit[%d]: got %v, want %v", i, decoded.Bits[i], b)
				}
			}
		})
	}
}

// TestDecodeBitStringErrors verifies DecodeBitStringValue rejects invalid input.
func TestDecodeBitStringErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty", input: []byte{}},
		{name: "invalid unused count", input: []byte{0x08}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeBitStringValue(tt.input)
			if !errors.Is(err, ErrDecodeFailure) {
				t.Errorf("want ErrDecodeFailure, got %v", err)
			}
		})
	}
}

// TestResultFlagsFromBitString verifies the three named result-flags bits.
func TestResultFlagsFromBitString(t *testing.T) {
	tests := []struct {
		name      string
		bits      []bool
		firstItem bool
		lastItem  bool
		moreItems bool
	}{
		{name: "none set", bits: []bool{false, false, false}, firstItem: false, lastItem: false, moreItems: false},
		{name: "first only", bits: []bool{true, false, false}, firstItem: true, lastItem: false, moreItems: false},
		{name: "last only", bits: []bool{false, true, false}, firstItem: false, lastItem: true, moreItems: false},
		{name: "more only", bits: []bool{false, false, true}, firstItem: false, lastItem: false, moreItems: true},
		{name: "all set", bits: []bool{true, true, true}, firstItem: true, lastItem: true, moreItems: true},
		{name: "empty bit string", bits: []bool{}, firstItem: false, lastItem: false, moreItems: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi, li, mi := ResultFlagsFromBitString(NewBitString(tt.bits))
			if fi != tt.firstItem || li != tt.lastItem || mi != tt.moreItems {
				t.Errorf("ResultFlagsFromBitString: got (%v,%v,%v), want (%v,%v,%v)",
					fi, li, mi, tt.firstItem, tt.lastItem, tt.moreItems)
			}
		})
	}
}

// TestEncodeDecodeDate verifies BACnetDate roundtrip including unspecified fields.
func TestEncodeDecodeDate(t *testing.T) {
	tests := []struct {
		name    string
		input   BACnetDate
		want    []byte
		wantErr bool
	}{
		{
			name:  "specific date",
			input: BACnetDate{Year: 2024, Month: 6, Day: 15, Weekday: 6},
			want:  []byte{byte(2024 - 1900), 6, 15, 6},
		},
		{
			name:  "unspecified year",
			input: BACnetDate{Year: BACnetDateUnspecified, Month: 1, Day: 1, Weekday: 1},
			want:  []byte{0xFF, 1, 1, 1},
		},
		{
			name:  "all unspecified",
			input: BACnetDate{Year: BACnetDateUnspecified, Month: BACnetDateUnspecified, Day: BACnetDateUnspecified, Weekday: BACnetDateUnspecified},
			want:  []byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			name:    "year before 1900",
			input:   BACnetDate{Year: 1899, Month: 1, Day: 1, Weekday: 1},
			wantErr: true,
		},
		{
			name:    "year out of range",
			input:   BACnetDate{Year: 2155, Month: 1, Day: 1, Weekday: 1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeDateValue(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrEncodeFailure) {
					t.Errorf("want ErrEncodeFailure, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EncodeDateValue: %v", err)
			}
			if !bytesEqual(encoded, tt.want) {
				t.Errorf("EncodeDateValue: got %v, want %v", encoded, tt.want)
			}

			decoded, err := DecodeDateValue(encoded)
			if err != nil {
				t.Fatalf("DecodeDateValue: %v", err)
			}
			if decoded != tt.input {
				t.Errorf("DecodeDateValue: got %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

// TestDecodeDateError verifies DecodeDateValue rejects wrong lengths.
func TestDecodeDateError(t *testing.T) {
	_, err := DecodeDateValue([]byte{1, 2, 3})
	if !errors.Is(err, ErrDecodeFailure) {
		t.Errorf("want ErrDecodeFailure, got %v", err)
	}
}

// TestEncodeDecodeTime verifies BACnetTime roundtrip.
func TestEncodeDecodeTime(t *testing.T) {
	tests := []struct {
		name  string
		input BACnetTime
		want  []byte
	}{
		{
			name:  "midnight",
			input: BACnetTime{Hour: 0, Minute: 0, Second: 0, Hundredths: 0},
			want:  []byte{0, 0, 0, 0},
		},
		{
			name:  "specific time",
			input: BACnetTime{Hour: 14, Minute: 30, Second: 45, Hundredths: 99},
			want:  []byte{14, 30, 45, 99},
		},
		{
			name:  "all unspecified",
			input: BACnetTime{Hour: BACnetTimeUnspecified, Minute: BACnetTimeUnspecified, Second: BACnetTimeUnspecified, Hundredths: BACnetTimeUnspecified},
			want:  []byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeTimeValue(tt.input)
			if !bytesEqual(encoded, tt.want) {
				t.Errorf("EncodeTimeValue: got %v, want %v", encoded, tt.want)
			}
			decoded, err := DecodeTimeValue(encoded)
			if err != nil {
				t.Fatalf("DecodeTimeValue: %v", err)
			}
			if decoded != tt.input {
				t.Errorf("DecodeTimeValue: got %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

// TestDecodeTimeError verifies DecodeTimeValue rejects wrong lengths.
func TestDecodeTimeError(t *testing.T) {
	_, err := DecodeTimeValue([]byte{1, 2, 3})
	if !errors.Is(err, ErrDecodeFailure) {
		t.Errorf("want ErrDecodeFailure, got %v", err)
	}
}

// TestEncodeDecodeDateTimeRoundtrip verifies full BACnetDateTime encode/decode.
func TestEncodeDecodeDateTimeRoundtrip(t *testing.T) {
	input := BACnetDateTime{
		Date: BACnetDate{Year: 2024, Month: 6, Day: 15, Weekday: 6},
		Time: BACnetTime{Hour: 10, Minute: 30, Second: 0, Hundredths: 0},
	}

	encoded, err := EncodeDateTimeValue(input)
	if err != nil {
		t.Fatalf("EncodeDateTimeValue: %v", err)
	}

	// Expect 10 bytes: Date (1 tag + 4 value) + Time (1 tag + 4 value) = 10
	if len(encoded) != 10 {
		t.Errorf("EncodeDateTimeValue length = %d, want 10", len(encoded))
	}

	decoded, err := DecodeDateTimeValue(encoded)
	if err != nil {
		t.Fatalf("DecodeDateTimeValue: %v", err)
	}
	if decoded.Date != input.Date {
		t.Errorf("Date: got %+v, want %+v", decoded.Date, input.Date)
	}
	if decoded.Time != input.Time {
		t.Errorf("Time: got %+v, want %+v", decoded.Time, input.Time)
	}
}

// TestDecodeDateTimeErrors verifies DecodeDateTimeValue rejects invalid input.
func TestDecodeDateTimeErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty", input: []byte{}},
		{name: "too short", input: []byte{0xA4, 0x7B, 0x06, 0x0F, 0x06}},
		{name: "trailing bytes", input: func() []byte {
			dt := BACnetDateTime{
				Date: BACnetDate{Year: 2024, Month: 1, Day: 1, Weekday: 1},
				Time: BACnetTime{},
			}
			b, _ := EncodeDateTimeValue(dt)
			return append(b, 0xFF)
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeDateTimeValue(tt.input)
			if !errors.Is(err, ErrDecodeFailure) {
				t.Errorf("want ErrDecodeFailure, got %v", err)
			}
		})
	}
}

// bytesEqual compares two byte slices for equality.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDecodeBooleanByte verifies the LVT-field helper.
func TestDecodeBooleanByte(t *testing.T) {
	if DecodeBooleanByte(0) != false {
		t.Error("DecodeBooleanByte(0) should be false")
	}
	if DecodeBooleanByte(1) != true {
		t.Error("DecodeBooleanByte(1) should be true")
	}
}

// TestDecodeBitStringValueUnusedExceedsData covers the error branch where
// unused bits count is valid (≤7) but the data bytes are missing.
func TestDecodeBitStringValueUnusedExceedsData(t *testing.T) {
	// unused=5 but no data byte follows → totalBits = 0*8-5 = -5
	_, err := DecodeBitStringValue([]byte{0x05})
	if !errors.Is(err, ErrDecodeFailure) {
		t.Errorf("want ErrDecodeFailure, got %v", err)
	}
}

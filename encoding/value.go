package encoding

import (
	"fmt"
	"math"

	"go.wdy.de/bacnet/common/types"
)

// EncodeUnsigned encodes a BACnet unsigned integer in the shortest form.
func EncodeUnsigned(v uint32) []byte {
	switch {
	case v <= 0xFF:
		return []byte{byte(v)}
	case v <= 0xFFFF:
		return []byte{byte(v >> 8), byte(v)}
	case v <= 0xFFFFFF:
		return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	default:
		return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

// DecodeUnsigned decodes a BACnet unsigned value from 1..4 bytes.
func DecodeUnsigned(raw []byte) (uint32, error) {
	if len(raw) == 0 || len(raw) > 4 {
		return 0, fmt.Errorf("%w: invalid unsigned length %d", ErrDecodeFailure, len(raw))
	}

	var out uint32
	for _, b := range raw {
		out = (out << 8) | uint32(b)
	}
	return out, nil
}

// EncodeApplicationPrimitive encodes one application primitive value.
func EncodeApplicationPrimitive(tagNumber uint8, value []byte) []byte {
	if len(value) <= 4 {
		out := make([]byte, 1+len(value))
		out[0] = (tagNumber << 4) | byte(len(value))
		copy(out[1:], value)
		return out
	}

	if len(value) <= 253 {
		out := make([]byte, 0, 2+len(value))
		out = append(out, (tagNumber<<4)|0x05, byte(len(value)))
		out = append(out, value...)
		return out
	}

	if len(value) <= 0xFFFF {
		out := make([]byte, 0, 4+len(value))
		out = append(out, (tagNumber<<4)|0x05, 254, byte(len(value)>>8), byte(len(value)))
		out = append(out, value...)
		return out
	}

	out := make([]byte, 0, 6+len(value))
	out = append(out, (tagNumber<<4)|0x05, 255, byte(len(value)>>24), byte(len(value)>>16), byte(len(value)>>8), byte(len(value)))
	out = append(out, value...)
	return out
}

// DecodeExpectedApplicationPrimitive decodes one application primitive at offset
// and validates the expected application tag number.
func DecodeExpectedApplicationPrimitive(payload []byte, offset int, expectedTag AppTag) (Tag, []byte, int, error) {
	if offset >= len(payload) {
		return Tag{}, nil, offset, fmt.Errorf("%w: missing application tag %d", ErrDecodeFailure, expectedTag)
	}

	tag, hdrLen, valueLen, err := ParseTag(payload[offset:])
	if err != nil {
		return Tag{}, nil, offset, fmt.Errorf("%w: decode application tag %d: %v", ErrDecodeFailure, expectedTag, err)
	}

	if tag.ContextSpecific || tag.TagNumber != expectedTag || tag.Opening || tag.Closing {
		return Tag{}, nil, offset, fmt.Errorf("%w: expected application tag %d", ErrDecodeFailure, expectedTag)
	}

	start := offset + hdrLen
	end := start + valueLen
	if end > len(payload) {
		return Tag{}, nil, offset, fmt.Errorf("%w: application tag %d length exceeds payload", ErrDecodeFailure, expectedTag)
	}

	return tag, append([]byte(nil), payload[start:end]...), end, nil
}

// EncodeObjectIdentifierValue encodes an ObjectIdentifier into a 4-byte value.
func EncodeObjectIdentifierValue(objectIdentifier types.ObjectIdentifier) []byte {
	raw := uint32(objectIdentifier)
	return []byte{byte(raw >> 24), byte(raw >> 16), byte(raw >> 8), byte(raw)}
}

// DecodeObjectIdentifierValue decodes a 4-byte object identifier value.
func DecodeObjectIdentifierValue(raw []byte) (types.ObjectIdentifier, error) {
	if len(raw) != 4 {
		return 0, fmt.Errorf("%w: invalid object-identifier length %d", ErrDecodeFailure, len(raw))
	}
	obj := types.ObjectIdentifier(uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3]))
	return obj, nil
}

// EncodeCharacterStringASCIIValue encodes an ASCII character-string value
// (character set 0 + bytes).
func EncodeCharacterStringASCIIValue(v string) ([]byte, error) {
	if !IsASCIIString(v) {
		return nil, fmt.Errorf("%w: non-ascii character-string", ErrEncodeFailure)
	}
	out := make([]byte, 0, len(v)+1)
	out = append(out, 0x00)
	out = append(out, []byte(v)...)
	return out, nil
}

// DecodeCharacterStringASCIIValue decodes an ASCII character-string value
// (character set 0 + bytes).
func DecodeCharacterStringASCIIValue(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("%w: empty character-string", ErrDecodeFailure)
	}
	if raw[0] != 0x00 {
		return "", fmt.Errorf("%w: unsupported character set %d", ErrDecodeFailure, raw[0])
	}
	v := string(raw[1:])
	if !IsASCIIString(v) {
		return "", fmt.Errorf("%w: non-ascii character-string", ErrDecodeFailure)
	}
	return v, nil
}

// IsASCIIString reports whether v only contains 7-bit ASCII bytes.
func IsASCIIString(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] > 0x7F {
			return false
		}
	}
	return true
}

// EncodeNull encodes a BACnet Null application primitive (tag 0, LVT=0).
func EncodeNull() []byte {
	return []byte{0x00}
}

// EncodeBoolean encodes a BACnet Boolean application primitive (tag 1).
// Boolean is special: the LVT field itself carries the value (0=false, 1=true),
// and no value bytes follow (clause 20.2.3).
func EncodeBoolean(v bool) []byte {
	if v {
		return []byte{(1 << 4) | 1}
	}
	return []byte{(1 << 4) | 0}
}

// EncodeBooleanValue encodes a BACnet Boolean as a 1-byte value (0x00=false, 0x01=true).
// Use this when encoding Boolean inside a context-specific primitive tag via
// EncodeContextPrimitive, where the value bytes are separate from the tag byte.
func EncodeBooleanValue(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

// DecodeBooleanByte decodes a BACnet Boolean value from the LVT field.
// raw must be the LVT nibble (lower 3 bits of the tag byte), not the full tag byte.
// Callers that have already parsed the tag header should use the LVT value directly.
// For convenience when the raw value byte is 0 or 1, use DecodeBooleanByte.
func DecodeBooleanByte(lvt byte) bool {
	return lvt != 0
}

// DecodeBoolean decodes a BACnet Boolean application primitive from a full tagged byte slice.
// raw must start with the Boolean tag byte; no value bytes follow.
// Returns the boolean value and the number of bytes consumed (always 1 on success).
func DecodeBoolean(raw []byte) (bool, int, error) {
	if len(raw) == 0 {
		return false, 0, fmt.Errorf("%w: missing boolean tag", ErrDecodeFailure)
	}
	b0 := raw[0]
	tagNibble := b0 >> 4
	contextSpecific := (b0>>3)&0x01 == 1
	lvt := b0 & 0x07
	if tagNibble != 1 || contextSpecific {
		return false, 0, fmt.Errorf("%w: expected boolean application tag (1), got tag nibble %d context=%t", ErrDecodeFailure, tagNibble, contextSpecific)
	}
	return lvt != 0, 1, nil
}

// EncodeSigned encodes a BACnet Signed Integer in the shortest two's-complement form (1–4 bytes).
// The value bytes only — no tag byte.
func EncodeSigned(v int32) []byte {
	switch {
	case v >= -0x80 && v <= 0x7F:
		return []byte{byte(v)}
	case v >= -0x8000 && v <= 0x7FFF:
		return []byte{byte(v >> 8), byte(v)}
	case v >= -0x800000 && v <= 0x7FFFFF:
		return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	default:
		return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

// DecodeSigned decodes a BACnet Signed Integer from 1..4 bytes (two's complement, big-endian).
func DecodeSigned(raw []byte) (int32, error) {
	if len(raw) == 0 || len(raw) > 4 {
		return 0, fmt.Errorf("%w: invalid signed length %d", ErrDecodeFailure, len(raw))
	}

	// Sign-extend from the MSB.
	var out int32
	if raw[0]&0x80 != 0 {
		out = -1 // all ones
	}
	for _, b := range raw {
		out = (out << 8) | int32(b)
	}
	return out, nil
}

// EncodeReal encodes a BACnet Real (IEEE 754 single-precision float) value bytes (4 bytes, no tag).
// NaN and ±Inf are valid on the wire per the BACnet standard (they carry physical meaning in some
// contexts), so no rejection is applied here.
func EncodeReal(v float32) []byte {
	bits := math.Float32bits(v)
	return []byte{byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits)}
}

// DecodeReal decodes a BACnet Real value from exactly 4 bytes.
func DecodeReal(raw []byte) (float32, error) {
	if len(raw) != 4 {
		return 0, fmt.Errorf("%w: invalid real length %d", ErrDecodeFailure, len(raw))
	}
	bits := uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
	return math.Float32frombits(bits), nil
}

// EncodeDouble encodes a BACnet Double (IEEE 754 double-precision float) value bytes (8 bytes, no tag).
func EncodeDouble(v float64) []byte {
	bits := math.Float64bits(v)
	return []byte{
		byte(bits >> 56), byte(bits >> 48), byte(bits >> 40), byte(bits >> 32),
		byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits),
	}
}

// DecodeDouble decodes a BACnet Double value from exactly 8 bytes.
func DecodeDouble(raw []byte) (float64, error) {
	if len(raw) != 8 {
		return 0, fmt.Errorf("%w: invalid double length %d", ErrDecodeFailure, len(raw))
	}
	bits := uint64(raw[0])<<56 | uint64(raw[1])<<48 | uint64(raw[2])<<40 | uint64(raw[3])<<32 |
		uint64(raw[4])<<24 | uint64(raw[5])<<16 | uint64(raw[6])<<8 | uint64(raw[7])
	return math.Float64frombits(bits), nil
}

// EncodeOctetStringValue encodes BACnet Octet String value bytes.
// The value bytes are the raw octets; the caller wraps them in an application or
// context tag via EncodeApplicationPrimitive / EncodeContextPrimitive.
func EncodeOctetStringValue(v []byte) []byte {
	out := make([]byte, len(v))
	copy(out, v)
	return out
}

// DecodeOctetStringValue decodes BACnet Octet String value bytes.
// The returned slice is a defensive copy of raw.
func DecodeOctetStringValue(raw []byte) []byte {
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

// EncodeEnumeratedValue encodes a BACnet Enumerated value in the shortest unsigned form (1–4 bytes).
// Enumerated is wire-identical to Unsigned (clause 20.2.11) but carries tag number 9.
// The value bytes only — no tag byte.
func EncodeEnumeratedValue(v uint32) []byte {
	return EncodeUnsigned(v)
}

// DecodeEnumeratedValue decodes a BACnet Enumerated value from 1..4 bytes.
func DecodeEnumeratedValue(raw []byte) (uint32, error) {
	return DecodeUnsigned(raw)
}

// BitString holds a BACnet Bit String value (clause 20.2.10).
// Bits is a slice of bool values, one per bit, in MSB-first order.
type BitString struct {
	// Bits holds the bit values in transmission order (index 0 = most significant bit of wire byte 1).
	Bits []bool
}

// NewBitString constructs a BitString from a slice of bool values.
func NewBitString(bits []bool) BitString {
	out := make([]bool, len(bits))
	copy(out, bits)
	return BitString{Bits: out}
}

// EncodeBitStringValue encodes a BACnet Bit String into value bytes (unused-bits count byte + data).
// The caller wraps the result in a tag via EncodeApplicationPrimitive / EncodeContextPrimitive.
func EncodeBitStringValue(v BitString) []byte {
	n := len(v.Bits)
	if n == 0 {
		return []byte{0x00}
	}

	numBytes := (n + 7) / 8
	unused := numBytes*8 - n
	data := make([]byte, numBytes)
	for i, bit := range v.Bits {
		if bit {
			data[i/8] |= 1 << (7 - uint(i%8))
		}
	}

	out := make([]byte, 1+numBytes)
	out[0] = byte(unused)
	copy(out[1:], data)
	return out
}

// DecodeBitStringValue decodes BACnet Bit String value bytes into a BitString.
// raw must be the value bytes (unused-bits count byte + data bytes).
func DecodeBitStringValue(raw []byte) (BitString, error) {
	if len(raw) == 0 {
		return BitString{}, fmt.Errorf("%w: empty bit string", ErrDecodeFailure)
	}
	unused := int(raw[0])
	dataBytes := raw[1:]
	if unused > 7 {
		return BitString{}, fmt.Errorf("%w: invalid unused-bits count %d", ErrDecodeFailure, unused)
	}
	totalBits := len(dataBytes)*8 - unused
	if totalBits < 0 {
		return BitString{}, fmt.Errorf("%w: bit string unused bits exceed data", ErrDecodeFailure)
	}
	bits := make([]bool, totalBits)
	for i := range bits {
		byteIdx := i / 8
		bitIdx := 7 - uint(i%8)
		bits[i] = (dataBytes[byteIdx]>>bitIdx)&1 == 1
	}
	return BitString{Bits: bits}, nil
}

// ResultFlagsFromBitString decodes the BACnet ResultFlags bit string into its three named bits:
// firstItem, lastItem, moreItems (bits 0, 1, 2 per clause 21.2.4.7).
func ResultFlagsFromBitString(v BitString) (firstItem, lastItem, moreItems bool) {
	get := func(i int) bool {
		if i < len(v.Bits) {
			return v.Bits[i]
		}
		return false
	}
	return get(0), get(1), get(2)
}

// BACnetDate represents a BACnet Date value (clause 20.2.12).
// Any field may be set to the special "unspecified" value (255).
type BACnetDate struct {
	// Year is the full year (e.g. 2024). Use 255 for "any year".
	Year uint16
	// Month is 1–12. Use 255 for "any month"; 13 = odd months, 14 = even months.
	Month uint8
	// Day is 1–31. Use 255 for "any day"; 32 = last day of month.
	Day uint8
	// Weekday is 1 (Monday) through 7 (Sunday). Use 255 for "any day of week".
	Weekday uint8
}

// BACnetDateUnspecified is the sentinel value for any BACnetDate field meaning "unspecified/any".
const BACnetDateUnspecified = 255

// EncodeDateValue encodes a BACnetDate into 4 value bytes (no tag).
// Year is encoded as (Year - 1900); years before 1900 are not representable.
// Precondition: Year >= 1900 unless Year == BACnetDateUnspecified.
func EncodeDateValue(v BACnetDate) ([]byte, error) {
	var yearByte byte
	if v.Year == BACnetDateUnspecified {
		yearByte = 255
	} else {
		if v.Year < 1900 {
			return nil, fmt.Errorf("%w: date year %d is before 1900", ErrEncodeFailure, v.Year)
		}
		encoded := v.Year - 1900
		if encoded > 254 {
			return nil, fmt.Errorf("%w: date year %d is out of BACnet range", ErrEncodeFailure, v.Year)
		}
		yearByte = byte(encoded)
	}
	return []byte{yearByte, v.Month, v.Day, v.Weekday}, nil
}

// DecodeDateValue decodes 4 value bytes into a BACnetDate.
func DecodeDateValue(raw []byte) (BACnetDate, error) {
	if len(raw) != 4 {
		return BACnetDate{}, fmt.Errorf("%w: invalid date length %d", ErrDecodeFailure, len(raw))
	}
	var year uint16
	if raw[0] == 255 {
		year = BACnetDateUnspecified
	} else {
		year = uint16(raw[0]) + 1900
	}
	return BACnetDate{Year: year, Month: raw[1], Day: raw[2], Weekday: raw[3]}, nil
}

// BACnetTime represents a BACnet Time value (clause 20.2.13).
// Any field may be set to the special "unspecified" value (255).
type BACnetTime struct {
	// Hour is 0–23. Use 255 for "any hour".
	Hour uint8
	// Minute is 0–59. Use 255 for "any minute".
	Minute uint8
	// Second is 0–59. Use 255 for "any second".
	Second uint8
	// Hundredths is 0–99. Use 255 for "any hundredths".
	Hundredths uint8
}

// BACnetTimeUnspecified is the sentinel value for any BACnetTime field meaning "unspecified/any".
const BACnetTimeUnspecified = 255

// EncodeTimeValue encodes a BACnetTime into 4 value bytes (no tag).
func EncodeTimeValue(v BACnetTime) []byte {
	return []byte{v.Hour, v.Minute, v.Second, v.Hundredths}
}

// DecodeTimeValue decodes 4 value bytes into a BACnetTime.
func DecodeTimeValue(raw []byte) (BACnetTime, error) {
	if len(raw) != 4 {
		return BACnetTime{}, fmt.Errorf("%w: invalid time length %d", ErrDecodeFailure, len(raw))
	}
	return BACnetTime{Hour: raw[0], Minute: raw[1], Second: raw[2], Hundredths: raw[3]}, nil
}

// BACnetDateTime represents a BACnet DateTime — a Date followed immediately by a Time.
// It is a constructed value used in ReadRange-by-time and other services.
type BACnetDateTime struct {
	Date BACnetDate
	Time BACnetTime
}

// EncodeDateTimeValue encodes a BACnetDateTime into 8 application-tagged bytes:
// a Date application tag (tag 10, 4 value bytes) followed by a Time application tag (tag 11, 4 value bytes).
func EncodeDateTimeValue(v BACnetDateTime) ([]byte, error) {
	dateBytes, err := EncodeDateValue(v.Date)
	if err != nil {
		return nil, err
	}
	timeBytes := EncodeTimeValue(v.Time)

	out := make([]byte, 0, 10)
	out = append(out, EncodeApplicationPrimitive(uint8(AppTagDate), dateBytes)...)
	out = append(out, EncodeApplicationPrimitive(uint8(AppTagTime), timeBytes)...)
	return out, nil
}

// DecodeDateTimeValue decodes 8 bytes (application Date tag + application Time tag) into a BACnetDateTime.
func DecodeDateTimeValue(raw []byte) (BACnetDateTime, error) {
	const dateTagNumber AppTag = 10
	const timeTagNumber AppTag = 11

	_, dateBytes, next, err := DecodeExpectedApplicationPrimitive(raw, 0, dateTagNumber)
	if err != nil {
		return BACnetDateTime{}, fmt.Errorf("%w: date part: %v", ErrDecodeFailure, err)
	}
	date, err := DecodeDateValue(dateBytes)
	if err != nil {
		return BACnetDateTime{}, err
	}

	_, timeBytes, end, err := DecodeExpectedApplicationPrimitive(raw, next, timeTagNumber)
	if err != nil {
		return BACnetDateTime{}, fmt.Errorf("%w: time part: %v", ErrDecodeFailure, err)
	}
	if end != len(raw) {
		return BACnetDateTime{}, fmt.Errorf("%w: trailing bytes after date-time", ErrDecodeFailure)
	}
	t, err := DecodeTimeValue(timeBytes)
	if err != nil {
		return BACnetDateTime{}, err
	}
	return BACnetDateTime{Date: date, Time: t}, nil
}

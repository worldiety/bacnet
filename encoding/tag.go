package encoding

import (
	"encoding/binary"
	"fmt"
	"slices"
)

// Tag describes one parsed BACnet tag header.
type Tag struct {
	TagNumber       AppTag
	ContextSpecific bool
	Opening         bool
	Closing         bool
}

// ParseTag parses one BACnet tag from raw and returns the parsed tag metadata,
// header length, and value length.
func ParseTag(raw []byte) (Tag, int, int, error) {
	if len(raw) == 0 {
		return Tag{}, 0, 0, fmt.Errorf("%w: empty tag", ErrDecodeFailure)
	}

	b0 := raw[0]
	tagNibble := uint32(b0 >> 4)
	contextSpecific := ((b0 >> 3) & 0x01) == 1
	lvt := b0 & 0x07
	offset := 1

	tagNumber := AppTag(tagNibble)
	if tagNibble == 0x0F {
		if len(raw) < 2 {
			return Tag{}, 0, 0, fmt.Errorf("%w: missing extended tag number", ErrDecodeFailure)
		}
		tagNumber = AppTag(raw[1])
		offset++
	}

	tag := Tag{TagNumber: tagNumber, ContextSpecific: contextSpecific}

	if contextSpecific && lvt == 6 {
		tag.Opening = true
		return tag, offset, 0, nil
	}
	if contextSpecific && lvt == 7 {
		tag.Closing = true
		return tag, offset, 0, nil
	}

	if !contextSpecific && tagNumber == 1 {
		// Boolean application tag encodes its value directly in LVT.
		return tag, offset, 0, nil
	}

	length := int(lvt)
	if lvt == 5 {
		if len(raw) < offset+1 {
			return Tag{}, 0, 0, fmt.Errorf("%w: missing extended length", ErrDecodeFailure)
		}
		ext := raw[offset]
		offset++
		switch {
		case ext <= 253:
			length = int(ext)
		case ext == 254:
			if len(raw) < offset+2 {
				return Tag{}, 0, 0, fmt.Errorf("%w: missing 2-byte extended length", ErrDecodeFailure)
			}
			length = int(binary.BigEndian.Uint16(raw[offset : offset+2]))
			offset += 2
		default:
			if len(raw) < offset+4 {
				return Tag{}, 0, 0, fmt.Errorf("%w: missing 4-byte extended length", ErrDecodeFailure)
			}
			length = int(binary.BigEndian.Uint32(raw[offset : offset+4]))
			offset += 4
		}
	}

	if len(raw) < offset+length {
		return Tag{}, 0, 0, fmt.Errorf("%w: tag value truncated", ErrDecodeFailure)
	}

	return tag, offset, length, nil
}

// EncodeOpeningTag encodes a BACnet opening tag header.
func EncodeOpeningTag(tagNumber uint8) []byte {
	if tagNumber <= 14 {
		return []byte{(tagNumber << 4) | 0x0E}
	}
	return []byte{0xFE, tagNumber}
}

// EncodeClosingTag encodes a BACnet closing tag header.
func EncodeClosingTag(tagNumber uint8) []byte {
	if tagNumber <= 14 {
		return []byte{(tagNumber << 4) | 0x0F}
	}
	return []byte{0xFF, tagNumber}
}

// EncodeContextPrimitive encodes one context-specific primitive tag/value.
func EncodeContextPrimitive(tagNumber uint8, value []byte) []byte {
	length := len(value)
	if tagNumber <= 14 && length <= 4 {
		out := make([]byte, 1+length)
		out[0] = (tagNumber << 4) | 0x08 | byte(length)
		copy(out[1:], value)
		return out
	}

	header := []byte{0xF0 | 0x08}
	header = append(header, tagNumber)
	switch {
	case length <= 253:
		header[0] |= 0x05
		header = append(header, byte(length))
	case length <= 0xFFFF:
		header[0] |= 0x05
		header = append(header, 254, byte(length>>8), byte(length))
	default:
		header[0] |= 0x05
		header = append(header, 255, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}

	out := make([]byte, 0, len(header)+length)
	out = append(out, header...)
	out = append(out, value...)
	return out
}

// LooksLikeContextPrimitiveTag reports whether b can represent the first byte
// of a short-form context-specific primitive tag with the given tag number.
func LooksLikeContextPrimitiveTag(b byte, tagNumber AppTag) bool {
	return ((b>>3)&0x01) == 1 && AppTag(b>>4) == tagNumber && (b&0x07) <= 5
}

// DecodeExpectedContextPrimitive decodes one context primitive at offset and
// validates the expected context tag number.
func DecodeExpectedContextPrimitive(payload []byte, offset int, expectedTag AppTag) (Tag, []byte, int, error) {
	if offset >= len(payload) {
		return Tag{}, nil, offset, fmt.Errorf("%w: missing context tag %d", ErrDecodeFailure, expectedTag)
	}

	tag, hdrLen, valueLen, err := ParseTag(payload[offset:])
	if err != nil {
		return Tag{}, nil, offset, fmt.Errorf("%w: decode context tag %d: %v", ErrDecodeFailure, expectedTag, err)
	}

	if !tag.ContextSpecific || tag.TagNumber != expectedTag || tag.Opening || tag.Closing {
		return Tag{}, nil, offset, fmt.Errorf("%w: expected context tag %d, got tag=%d context=%t opening=%t closing=%t", ErrDecodeFailure, expectedTag, tag.TagNumber, tag.ContextSpecific, tag.Opening, tag.Closing)
	}

	start := offset + hdrLen
	end := start + valueLen
	if end > len(payload) {
		return Tag{}, nil, offset, fmt.Errorf("%w: context tag %d length exceeds payload", ErrDecodeFailure, expectedTag)
	}

	return tag, slices.Clone(payload[start:end]), end, nil
}

// ExpectOpeningTag validates and consumes an opening tag at offset.
func ExpectOpeningTag(payload []byte, offset int, tagNumber AppTag) (int, error) {
	if offset >= len(payload) {
		return offset, fmt.Errorf("%w: missing opening tag %d", ErrDecodeFailure, tagNumber)
	}
	tag, hdrLen, _, err := ParseTag(payload[offset:])
	if err != nil {
		return offset, fmt.Errorf("%w: decode opening tag %d: %v", ErrDecodeFailure, tagNumber, err)
	}
	if !tag.Opening || tag.TagNumber != tagNumber {
		return offset, fmt.Errorf("%w: expected opening tag %d", ErrDecodeFailure, tagNumber)
	}
	return offset + hdrLen, nil
}

// ExpectClosingTag validates and consumes a closing tag at offset.
func ExpectClosingTag(payload []byte, offset int, tagNumber AppTag) (int, error) {
	if offset >= len(payload) {
		return offset, fmt.Errorf("%w: missing closing tag %d", ErrDecodeFailure, tagNumber)
	}
	tag, hdrLen, _, err := ParseTag(payload[offset:])
	if err != nil {
		return offset, fmt.Errorf("%w: decode closing tag %d: %v", ErrDecodeFailure, tagNumber, err)
	}
	if !tag.Closing || tag.TagNumber != tagNumber {
		return offset, fmt.Errorf("%w: expected closing tag %d", ErrDecodeFailure, tagNumber)
	}
	return offset + hdrLen, nil
}

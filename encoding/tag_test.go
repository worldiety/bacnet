package encoding

import (
	"errors"
	"testing"
)

func TestParseTag(t *testing.T) {
	tests := []struct {
		name       string
		raw        []byte
		wantTag    Tag
		wantHdrLen int
		wantValLen int
		wantErr    error
	}{
		{
			name:       "context primitive short",
			raw:        []byte{0x19, 0xAA},
			wantTag:    Tag{TagNumber: 1, ContextSpecific: true},
			wantHdrLen: 1,
			wantValLen: 1,
		},
		{
			name:       "opening tag",
			raw:        []byte{0x3E},
			wantTag:    Tag{TagNumber: 3, ContextSpecific: true, Opening: true},
			wantHdrLen: 1,
			wantValLen: 0,
		},
		{
			name:       "closing tag",
			raw:        []byte{0x3F},
			wantTag:    Tag{TagNumber: 3, ContextSpecific: true, Closing: true},
			wantHdrLen: 1,
			wantValLen: 0,
		},
		{
			name:       "extended length 2 byte",
			raw:        []byte{0x25, 254, 0x00, 0x06, 1, 2, 3, 4, 5, 6},
			wantTag:    Tag{TagNumber: 2},
			wantHdrLen: 4,
			wantValLen: 6,
		},
		{
			name:    "empty",
			raw:     nil,
			wantErr: ErrDecodeFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag, hdrLen, valLen, err := ParseTag(tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if tag != tt.wantTag {
				t.Fatalf("tag = %+v, want %+v", tag, tt.wantTag)
			}
			if hdrLen != tt.wantHdrLen {
				t.Fatalf("hdrLen = %d, want %d", hdrLen, tt.wantHdrLen)
			}
			if valLen != tt.wantValLen {
				t.Fatalf("valLen = %d, want %d", valLen, tt.wantValLen)
			}
		})
	}
}

func TestEncodeContextPrimitiveAndDecodeExpectedContextPrimitiveRoundtrip(t *testing.T) {
	value := []byte{0xAA, 0xBB, 0xCC}
	encoded := EncodeContextPrimitive(4, value)

	tag, decoded, next, err := DecodeExpectedContextPrimitive(encoded, 0, 4)
	if err != nil {
		t.Fatalf("DecodeExpectedContextPrimitive: %v", err)
	}

	if !tag.ContextSpecific || tag.TagNumber != 4 {
		t.Fatalf("tag = %+v", tag)
	}
	if next != len(encoded) {
		t.Fatalf("next = %d, want %d", next, len(encoded))
	}
	if len(decoded) != len(value) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(value))
	}
	for i := range value {
		if decoded[i] != value[i] {
			t.Fatalf("decoded[%d] = %02x, want %02x", i, decoded[i], value[i])
		}
	}
}

func TestExpectOpeningAndClosingTag(t *testing.T) {
	payload := append(EncodeOpeningTag(3), EncodeClosingTag(3)...)

	next, err := ExpectOpeningTag(payload, 0, 3)
	if err != nil {
		t.Fatalf("ExpectOpeningTag: %v", err)
	}
	if next != 1 {
		t.Fatalf("next = %d, want 1", next)
	}

	end, err := ExpectClosingTag(payload, next, 3)
	if err != nil {
		t.Fatalf("ExpectClosingTag: %v", err)
	}
	if end != len(payload) {
		t.Fatalf("end = %d, want %d", end, len(payload))
	}
}

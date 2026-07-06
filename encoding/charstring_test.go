package encoding

import (
	"errors"
	"testing"
	"unicode/utf16"
)

// utf16beValue builds a character-string value (charset octet + UTF-16BE body)
// for character set 4 (UCS-2).
func utf16beValue(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, 0, 1+len(units)*2)
	out = append(out, byte(CharacterSetUCS2))
	for _, u := range units {
		out = append(out, byte(u>>8), byte(u))
	}
	return out
}

// utf32beValue builds a character-string value (charset octet + UTF-32BE body)
// for character set 5 (UCS-4).
func utf32beValue(s string) []byte {
	out := []byte{byte(CharacterSetUCS4)}
	for _, r := range s {
		cp := uint32(r)
		out = append(out, byte(cp>>24), byte(cp>>16), byte(cp>>8), byte(cp))
	}
	return out
}

// TestDecodeCharacterStringValueCharsets verifies the charset-aware decoder for
// each supported character set, including the UCS-2 case Kieback&Peter emits.
func TestDecodeCharacterStringValueCharsets(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{
			name: "utf8 ascii",
			raw:  append([]byte{byte(CharacterSetUTF8)}, []byte("hi!")...),
			want: "hi!",
		},
		{
			name: "utf8 accented",
			raw:  append([]byte{byte(CharacterSetUTF8)}, []byte("Küche")...),
			want: "Küche",
		},
		{
			name: "utf8 with latin1 bytes falls back",
			raw:  []byte{byte(CharacterSetUTF8), 0xFC, 0x78}, // 0xFC='ü' latin1, 'x'
			want: "üx",
		},
		{
			name: "ucs2 kieback peter",
			raw:  utf16beValue("Außentemperatur"),
			want: "Außentemperatur",
		},
		{
			name: "ucs2 with surrogate pair (emoji)",
			raw:  utf16beValue("A😀"),
			want: "A😀",
		},
		{
			name: "ucs4 ascii+accent",
			raw:  utf32beValue("Aü"),
			want: "Aü",
		},
		{
			name: "ucs4 emoji",
			raw:  utf32beValue("A😀"),
			want: "A😀",
		},
		{
			name: "iso-8859-1",
			raw:  []byte{byte(CharacterSetISO8859_1), 0xC4, 0xFC}, // 'Ä','ü' latin1
			want: "Äü",
		},
		{
			name: "empty body charset 0",
			raw:  []byte{byte(CharacterSetUTF8)},
			want: "",
		},
		{
			name: "empty body ucs2",
			raw:  []byte{byte(CharacterSetUCS2)},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeCharacterStringValue(tt.raw)
			if err != nil {
				t.Fatalf("DecodeCharacterStringValue: unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDecodeCharacterStringValueFallback verifies that rare or malformed inputs
// never error but fall back to a readable Latin-1 interpretation.
func TestDecodeCharacterStringValueFallback(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{
			name: "dbcs set 1 non-utf8 -> latin1",
			raw:  []byte{byte(CharacterSetDBCS), 0xE4}, // 'ä' latin1, invalid utf8
			want: "ä",
		},
		{
			name: "jis set 2 valid-utf8 body kept",
			raw:  append([]byte{byte(CharacterSetJISX0208)}, []byte("ok")...),
			want: "ok",
		},
		{
			name: "unknown charset 99 non-utf8 -> latin1",
			raw:  []byte{99, 0xFF},
			want: "ÿ",
		},
		{
			name: "ucs2 odd trailing byte -> latin1 tail, no data lost",
			raw:  []byte{byte(CharacterSetUCS2), 0x00, 0x41, 0x42}, // "A" + stray 0x42
			want: "AB",
		},
		{
			name: "ucs4 short trailing bytes -> latin1 tail",
			raw:  []byte{byte(CharacterSetUCS4), 0x00, 0x00, 0x00, 0x41, 0x42}, // "A" + stray 0x42
			want: "AB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeCharacterStringValue(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDecodeCharacterStringValueEmpty verifies the only error case: a
// zero-length value (missing the required character-set octet).
func TestDecodeCharacterStringValueEmpty(t *testing.T) {
	_, err := DecodeCharacterStringValue(nil)
	if err == nil {
		t.Fatal("want error for empty value, got nil")
	}
	if !errors.Is(err, ErrDecodeFailure) {
		t.Errorf("error should wrap ErrDecodeFailure, got: %v", err)
	}
}

// TestEncodeCharacterStringValue verifies encoding emits charset 0 (UTF-8) and
// rejects non-UTF-8 Go strings.
func TestEncodeCharacterStringValue(t *testing.T) {
	enc, err := EncodeCharacterStringValue("Zürich")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if enc[0] != byte(CharacterSetUTF8) {
		t.Fatalf("charset octet = %d, want 0 (UTF-8)", enc[0])
	}
	if string(enc[1:]) != "Zürich" {
		t.Fatalf("body = %q, want Zürich", string(enc[1:]))
	}

	if _, err := EncodeCharacterStringValue("\xFF bad"); err == nil {
		t.Fatal("want error for invalid-UTF-8 input, got nil")
	} else if !errors.Is(err, ErrEncodeFailure) {
		t.Errorf("error should wrap ErrEncodeFailure, got: %v", err)
	}
}

// TestCharacterStringUCS2ThroughApplicationValue verifies the full path used by
// the client: a UCS-2 application-tagged value decodes to a proper
// AppCharacterString via DecodeApplicationValue (no recovery path needed).
func TestCharacterStringUCS2ThroughApplicationValue(t *testing.T) {
	raw := EncodeApplicationPrimitive(uint8(AppTagCharacterString), utf16beValue("Außentemperatur"))
	val, end, err := DecodeApplicationValue(raw, 0)
	if err != nil {
		t.Fatalf("DecodeApplicationValue: %v", err)
	}
	if end != len(raw) {
		t.Fatalf("end = %d, want %d", end, len(raw))
	}
	cs, ok := val.(AppCharacterString)
	if !ok {
		t.Fatalf("decoded type %T, want AppCharacterString", val)
	}
	if string(cs) != "Außentemperatur" {
		t.Fatalf("got %q, want Außentemperatur", string(cs))
	}
}

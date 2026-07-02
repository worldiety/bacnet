package client

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/worldiety/bacnet/common/types"
	"github.com/worldiety/bacnet/encoding"
)

// PropertyValue is an ergonomic wrapper around a decoded BACnet application
// value. It exposes typed accessors so callers rarely need to type-switch on
// the underlying encoding.ApplicationValue themselves.
//
// If the device returned a value the library could not decode (e.g. a
// non-conformant character set), Raw is nil and RawBytes holds the original
// application-tagged bytes so nothing is lost.
type PropertyValue struct {
	// Raw is the decoded value, or nil if decoding failed.
	Raw encoding.ApplicationValue
	// RawBytes holds the original application-tagged bytes.
	RawBytes []byte
}

// Decoded reports whether the value was successfully decoded.
func (v PropertyValue) Decoded() bool { return v.Raw != nil }

// IsNull reports whether the value is a BACnet Null.
func (v PropertyValue) IsNull() bool {
	_, ok := v.Raw.(encoding.AppNull)
	return ok
}

// Float64 returns the value as a float64 when it is a Real or Double.
func (v PropertyValue) Float64() (float64, bool) {
	switch t := v.Raw.(type) {
	case encoding.AppReal:
		return float64(float32(t)), true
	case encoding.AppDouble:
		return float64(t), true
	default:
		return 0, false
	}
}

// Uint returns the value as a uint32 when it is an Unsigned Integer or Enum.
func (v PropertyValue) Uint() (uint32, bool) {
	switch t := v.Raw.(type) {
	case encoding.AppUnsignedInteger:
		return uint32(t), true
	case encoding.AppEnum:
		return uint32(t), true
	default:
		return 0, false
	}
}

// Int returns the value as an int32 when it is a Signed Integer.
func (v PropertyValue) Int() (int32, bool) {
	if t, ok := v.Raw.(encoding.AppInteger); ok {
		return int32(t), true
	}
	return 0, false
}

// Bool returns the value as a bool when it is a Boolean.
func (v PropertyValue) Bool() (bool, bool) {
	if t, ok := v.Raw.(encoding.AppBoolean); ok {
		return bool(t), true
	}
	return false, false
}

// Text returns the value as a string when it is a Character String.
func (v PropertyValue) Text() (string, bool) {
	if t, ok := v.Raw.(encoding.AppCharacterString); ok {
		return string(t), true
	}
	// Best-effort recovery for non-conformant charsets.
	if v.Raw == nil && len(v.RawBytes) > 0 {
		if s, ok := recoverCharacterString(v.RawBytes); ok {
			return s, true
		}
	}
	return "", false
}

// ObjectID returns the value as an object identifier when it is one.
func (v PropertyValue) ObjectID() (types.ObjectIdentifier, bool) {
	if t, ok := v.Raw.(encoding.AppObjectIdentifier); ok {
		return types.ObjectIdentifier(t), true
	}
	return 0, false
}

// Display renders the value as a human-readable string. pid provides context
// so that, for example, a units enumeration or an object-type is rendered with
// its name. Pass a zero PropertyIdentifier when no context is available.
func (v PropertyValue) Display(pid types.PropertyIdentifier) string {
	if v.Raw == nil {
		if s, ok := recoverCharacterString(v.RawBytes); ok {
			return s
		}
		if len(v.RawBytes) == 0 {
			return "(empty)"
		}
		return fmt.Sprintf("(raw 0x%s)", hex.EncodeToString(v.RawBytes))
	}
	return formatValue(v.Raw, pid)
}

// String renders the value without property context.
func (v PropertyValue) String() string { return v.Display(0) }

// itoa32 formats a uint32 as a decimal string.
func itoa32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// formatValue renders a decoded BACnet application value as a readable string.
func formatValue(v encoding.ApplicationValue, pid types.PropertyIdentifier) string {
	switch val := v.(type) {
	case encoding.AppNull:
		return "null"
	case encoding.AppBoolean:
		return strconv.FormatBool(bool(val))
	case encoding.AppUnsignedInteger:
		if pid == types.PropertyIdentifierUnits {
			return fmt.Sprintf("%d (%s)", uint32(val), UnitsName(uint32(val)))
		}
		if pid == types.PropertyIdentifierObjectType {
			return fmt.Sprintf("%d (%s)", uint32(val), ObjectTypeName(types.ObjectType(val)))
		}
		return itoa32(uint32(val))
	case encoding.AppInteger:
		return strconv.FormatInt(int64(int32(val)), 10)
	case encoding.AppReal:
		return strconv.FormatFloat(float64(float32(val)), 'g', -1, 32)
	case encoding.AppDouble:
		return strconv.FormatFloat(float64(val), 'g', -1, 64)
	case encoding.AppOctetString:
		return "0x" + hex.EncodeToString([]byte(val))
	case encoding.AppCharacterString:
		return strconv.Quote(string(val))
	case encoding.AppBitString:
		return formatBits(val.Bits)
	case encoding.AppEnum:
		if pid == types.PropertyIdentifierUnits {
			return fmt.Sprintf("%d (%s)", uint32(val), UnitsName(uint32(val)))
		}
		if pid == types.PropertyIdentifierObjectType {
			return fmt.Sprintf("%d (%s)", uint32(val), ObjectTypeName(types.ObjectType(val)))
		}
		return fmt.Sprintf("enum(%d)", uint32(val))
	case encoding.AppDate:
		return fmt.Sprintf("%04d-%02d-%02d (weekday %d)", val.Year, val.Month, val.Day, val.Weekday)
	case encoding.AppTime:
		return fmt.Sprintf("%02d:%02d:%02d.%02d", val.Hour, val.Minute, val.Second, val.Hundredths)
	case encoding.AppObjectIdentifier:
		oid := types.ObjectIdentifier(val)
		return fmt.Sprintf("%s:%d", ObjectTypeName(oid.ObjectType()), oid.Instance())
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatBits(bits []bool) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, bit := range bits {
		if i > 0 {
			b.WriteByte(' ')
		}
		if bit {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	b.WriteByte('}')
	return b.String()
}

// recoverCharacterString attempts to decode a single application-tagged
// character string (tag 7) from raw as a last resort. The library already
// decodes valid UTF-8 in character set 0; this handles non-conformant devices
// that place Latin-1 (or otherwise non-UTF-8) bytes in charset 0. It returns
// the string and true on success.
func recoverCharacterString(raw []byte) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	tag, hLen, vLen, err := encoding.ParseTag(raw)
	if err != nil {
		return "", false
	}
	if tag.ContextSpecific || tag.TagNumber != encoding.AppTagCharacterString {
		return "", false
	}
	if hLen+vLen > len(raw) || vLen < 1 {
		return "", false
	}
	v := raw[hLen : hLen+vLen]
	charset := v[0]
	body := v[1:]

	switch charset {
	case 0: // UTF-8 per spec; some devices emit Latin-1 here.
		if utf8.Valid(body) {
			return string(body), true
		}
		// Fall back to Latin-1 interpretation for stray high bytes.
		return latin1(body), true
	default:
		return "", false
	}
}

// latin1 decodes bytes as ISO-8859-1 into a UTF-8 Go string.
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// ParseValue parses a "<type>:<value>" string into an application value ready
// to be written. This is convenient for CLIs and config files; programmatic
// callers may prefer the typed Value* constructors.
//
// Supported types:
//
//	null:                 -> Null (used to relinquish a commandable property)
//	bool:true|false       -> Boolean
//	unsigned:<n>          -> Unsigned Integer
//	int:<n>               -> Signed Integer
//	real:<f>              -> Real (single precision)
//	double:<f>            -> Double
//	enum:<n>              -> Enumerated
//	string:<text>         -> Character String (UTF-8)
//	octet:<hex>           -> Octet String (hex bytes, e.g. octet:0a1b2c)
//	oid:<type>:<instance> -> Object Identifier
func ParseValue(s string) (encoding.ApplicationValue, error) {
	kind, rest, ok := strings.Cut(s, ":")
	if !ok {
		return nil, fmt.Errorf("invalid value %q: expected <type>:<value> (e.g. real:21.5, null:)", s)
	}
	kind = strings.ToLower(strings.TrimSpace(kind))

	switch kind {
	case "null":
		return encoding.AppNull{}, nil

	case "bool", "boolean":
		b, err := strconv.ParseBool(strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("invalid bool %q: %w", rest, err)
		}
		return encoding.AppBoolean(b), nil

	case "unsigned", "uint", "u":
		n, err := strconv.ParseUint(strings.TrimSpace(rest), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid unsigned %q: %w", rest, err)
		}
		return encoding.AppUnsignedInteger(uint32(n)), nil

	case "int", "integer", "signed":
		n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", rest, err)
		}
		return encoding.AppInteger(int32(n)), nil

	case "real", "float":
		f, err := strconv.ParseFloat(strings.TrimSpace(rest), 32)
		if err != nil {
			return nil, fmt.Errorf("invalid real %q: %w", rest, err)
		}
		return encoding.AppReal(float32(f)), nil

	case "double":
		f, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid double %q: %w", rest, err)
		}
		return encoding.AppDouble(f), nil

	case "enum", "enumerated":
		n, err := strconv.ParseUint(strings.TrimSpace(rest), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid enum %q: %w", rest, err)
		}
		return encoding.AppEnum(uint32(n)), nil

	case "string", "str", "chars":
		return encoding.AppCharacterString(rest), nil

	case "octet", "octetstring", "hex":
		clean := strings.ReplaceAll(strings.TrimSpace(rest), " ", "")
		clean = strings.TrimPrefix(clean, "0x")
		b, err := hex.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("invalid octet-string hex %q: %w", rest, err)
		}
		return encoding.AppOctetString(b), nil

	case "oid", "objectid", "object-identifier":
		obj, err := ParseObject(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid object-identifier %q: %w", rest, err)
		}
		return encoding.AppObjectIdentifier(obj.OID()), nil

	default:
		return nil, fmt.Errorf("unknown value type %q (supported: null bool unsigned int real double enum string octet oid)", kind)
	}
}

// Typed value constructors for programmatic callers.

// ValueNull returns a BACnet Null value.
func ValueNull() encoding.ApplicationValue { return encoding.AppNull{} }

// ValueBool returns a BACnet Boolean value.
func ValueBool(b bool) encoding.ApplicationValue { return encoding.AppBoolean(b) }

// ValueUnsigned returns a BACnet Unsigned Integer value.
func ValueUnsigned(v uint32) encoding.ApplicationValue { return encoding.AppUnsignedInteger(v) }

// ValueInt returns a BACnet Signed Integer value.
func ValueInt(v int32) encoding.ApplicationValue { return encoding.AppInteger(v) }

// ValueReal returns a BACnet Real (single-precision) value.
func ValueReal(v float32) encoding.ApplicationValue { return encoding.AppReal(v) }

// ValueDouble returns a BACnet Double value.
func ValueDouble(v float64) encoding.ApplicationValue { return encoding.AppDouble(v) }

// ValueEnum returns a BACnet Enumerated value.
func ValueEnum(v uint32) encoding.ApplicationValue { return encoding.AppEnum(v) }

// ValueString returns a BACnet Character String value (UTF-8).
func ValueString(s string) encoding.ApplicationValue { return encoding.AppCharacterString(s) }

// ValueObjectID returns a BACnet Object Identifier value.
func ValueObjectID(oid types.ObjectIdentifier) encoding.ApplicationValue {
	return encoding.AppObjectIdentifier(oid)
}

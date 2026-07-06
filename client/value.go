package client

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/worldiety/bacnet/common/types"
	"github.com/worldiety/bacnet/encoding"
)

// PropertyValue is an ergonomic wrapper around a decoded BACnet application
// value. It exposes typed accessors so callers rarely need to type-switch on
// the underlying encoding.ApplicationValue themselves.
//
// A property value on the wire may hold a single application value (e.g. a Real
// present-value) or a sequence of them (a list-valued property such as
// object-list, state-text, priority-array or property-list). Raw is always the
// first decoded value; Values holds every decoded value in order. For scalar
// properties len(Values) == 1.
//
// If the device returned a value the library could not decode, Raw is nil,
// Values is empty and RawBytes holds the original application-tagged bytes so
// nothing is lost. Character strings in any standard character set (UTF-8,
// ISO-8859-1, UCS-2/UTF-16BE, UCS-4/UTF-32BE) decode successfully, so Text()
// and Display() work without a recovery path; use Charset to inspect the
// original encoding.
type PropertyValue struct {
	// Raw is the first decoded value, or nil if decoding failed. For a
	// list-valued property it is the first element (equal to Values[0]).
	Raw encoding.ApplicationValue
	// Values holds every decoded application value in wire order. It has one
	// element for a scalar property, several for a list-valued property, and is
	// empty when decoding failed.
	Values []encoding.ApplicationValue
	// RawBytes holds the original application-tagged bytes.
	RawBytes []byte
}

// Decoded reports whether the value was successfully decoded.
func (v PropertyValue) Decoded() bool { return v.Raw != nil }

// Len reports how many application values were decoded. It is 0 for an
// undecodable value, 1 for a scalar property, and more for a list-valued one.
func (v PropertyValue) Len() int { return len(v.Values) }

// IsList reports whether the value holds more than one application value (a
// list-valued property such as object-list or state-text).
func (v PropertyValue) IsList() bool { return len(v.Values) > 1 }

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
//
// The library decodes the standard BACnet character sets (UTF-8, ISO-8859-1,
// UCS-2/UTF-16BE and UCS-4/UTF-32BE), so Raw is a proper AppCharacterString for
// conformant and common non-conformant devices alike and this accessor works
// without any caller-side recovery. For a list-valued property it returns the
// first string; use Values to reach the rest.
func (v PropertyValue) Text() (string, bool) {
	if t, ok := v.Raw.(encoding.AppCharacterString); ok {
		return string(t), true
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

// ObjectIDs returns every value as an object identifier. It is the natural
// accessor for list-valued reference properties such as object-list and
// property-list. It returns false if the value is undecodable or any element is
// not an object identifier.
func (v PropertyValue) ObjectIDs() ([]types.ObjectIdentifier, bool) {
	if len(v.Values) == 0 {
		return nil, false
	}
	out := make([]types.ObjectIdentifier, len(v.Values))
	for i, e := range v.Values {
		oid, ok := e.(encoding.AppObjectIdentifier)
		if !ok {
			return nil, false
		}
		out[i] = types.ObjectIdentifier(oid)
	}
	return out, true
}

// Charset returns the BACnet character set of a Character String value as it
// appeared on the wire (the leading octet of the value, per clause 20.2.9),
// together with true. It reports false for values that are not character
// strings or whose raw bytes are unavailable. Text() and Display() already
// decode every standard set to UTF-8; Charset is for callers that need to know
// or record the original encoding.
func (v PropertyValue) Charset() (encoding.CharacterSet, bool) {
	if len(v.RawBytes) == 0 {
		return 0, false
	}
	tag, hLen, vLen, err := encoding.ParseTag(v.RawBytes)
	if err != nil || tag.ContextSpecific || tag.TagNumber != encoding.AppTagCharacterString {
		return 0, false
	}
	if vLen < 1 || hLen+vLen > len(v.RawBytes) {
		return 0, false
	}
	return encoding.CharacterSet(v.RawBytes[hLen]), true
}

// Display renders the value as a human-readable string. pid provides context
// so that, for example, a units enumeration or an object-type is rendered with
// its name. Pass a zero PropertyIdentifier when no context is available.
//
// A list-valued property is rendered as "[a, b, c]"; a scalar renders as the
// bare value.
func (v PropertyValue) Display(pid types.PropertyIdentifier) string {
	if v.Raw == nil {
		if len(v.RawBytes) == 0 {
			return "(empty)"
		}
		return fmt.Sprintf("(raw 0x%s)", hex.EncodeToString(v.RawBytes))
	}
	if len(v.Values) <= 1 {
		return formatValue(v.Raw, pid)
	}
	parts := make([]string, len(v.Values))
	for i, e := range v.Values {
		parts[i] = formatValue(e, pid)
	}
	return "[" + strings.Join(parts, ", ") + "]"
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

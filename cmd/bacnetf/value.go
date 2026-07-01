package main

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/worldiety/bacnet/common/types"
	bacencoding "github.com/worldiety/bacnet/encoding"
)

// itoa32 is a tiny helper to format a uint32 as a decimal string.
func itoa32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// formatValue renders a decoded BACnet application value as a readable string.
//
// pid is the property the value belongs to; it is used only to add contextual
// hints (e.g. decoding a units enumeration into a unit name). Pass a zero value
// if no context is available.
func formatValue(v bacencoding.ApplicationValue, pid types.PropertyIdentifier) string {
	switch val := v.(type) {
	case bacencoding.AppNull:
		return "null"
	case bacencoding.AppBoolean:
		return strconv.FormatBool(bool(val))
	case bacencoding.AppUnsignedInteger:
		if pid == propByNameOrZero("units") {
			return fmt.Sprintf("%d (%s)", uint32(val), unitsName(uint32(val)))
		}
		if pid == types.PropertyIdentifierObjectType {
			return fmt.Sprintf("%d (%s)", uint32(val), objectTypeName(types.ObjectType(val)))
		}
		return itoa32(uint32(val))
	case bacencoding.AppInteger:
		return strconv.FormatInt(int64(int32(val)), 10)
	case bacencoding.AppReal:
		return strconv.FormatFloat(float64(float32(val)), 'g', -1, 32)
	case bacencoding.AppDouble:
		return strconv.FormatFloat(float64(val), 'g', -1, 64)
	case bacencoding.AppOctetString:
		return "0x" + hex.EncodeToString([]byte(val))
	case bacencoding.AppCharacterString:
		return strconv.Quote(string(val))
	case bacencoding.AppBitString:
		return formatBits(val.Bits)
	case bacencoding.AppEnum:
		if pid == propByNameOrZero("units") {
			return fmt.Sprintf("%d (%s)", uint32(val), unitsName(uint32(val)))
		}
		if pid == types.PropertyIdentifierObjectType {
			return fmt.Sprintf("%d (%s)", uint32(val), objectTypeName(types.ObjectType(val)))
		}
		return fmt.Sprintf("enum(%d)", uint32(val))
	case bacencoding.AppDate:
		return fmt.Sprintf("%04d-%02d-%02d (weekday %d)", val.Year, val.Month, val.Day, val.Weekday)
	case bacencoding.AppTime:
		return fmt.Sprintf("%02d:%02d:%02d.%02d", val.Hour, val.Minute, val.Second, val.Hundredths)
	case bacencoding.AppObjectIdentifier:
		oid := types.ObjectIdentifier(val)
		return fmt.Sprintf("%s:%d", objectTypeName(oid.ObjectType()), oid.Instance())
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

// propByNameOrZero returns the property id for a known name, or 0 if unknown.
// Used only for display-context checks.
func propByNameOrZero(name string) types.PropertyIdentifier {
	if pid, ok := propertyByName[name]; ok {
		return pid
	}
	return 0
}

// decodeAndFormat decodes a raw application-tagged value and formats it.
//
// The library decodes character set 0 as UTF-8 (the standard charset), so
// ASCII and UTF-8 strings decode directly. As a last resort, if the library
// still cannot decode a character string (e.g. a non-conformant device that
// emits Latin-1 bytes in charset 0), we attempt a best-effort recovery before
// falling back to a hex dump so the operator always sees something usable.
func decodeAndFormat(raw []byte, pid types.PropertyIdentifier) string {
	if len(raw) == 0 {
		return "(empty)"
	}
	val, _, err := bacencoding.DecodeApplicationValue(raw, 0)
	if err != nil {
		if s, ok := recoverCharacterString(raw); ok {
			return s
		}
		return fmt.Sprintf("(raw 0x%s)", hex.EncodeToString(raw))
	}
	return formatValue(val, pid)
}

// recoverCharacterString attempts to decode a single application-tagged
// character string (tag 7) from raw as a last resort. The library already
// decodes valid UTF-8 in character set 0; this handles non-conformant devices
// that place Latin-1 (or otherwise non-UTF-8) bytes in charset 0. It returns
// the quoted string and true on success.
func recoverCharacterString(raw []byte) (string, bool) {
	tag, hLen, vLen, err := bacencoding.ParseTag(raw)
	if err != nil {
		return "", false
	}
	if tag.ContextSpecific || tag.TagNumber != bacencoding.AppTagCharacterString {
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
			return strconv.Quote(string(body)), true
		}
		// Fall back to Latin-1 interpretation for stray high bytes.
		return strconv.Quote(latin1(body)), true
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

// parseWriteValue parses a "<type>:<value>" argument into an application value
// ready to be encoded for a WriteProperty request.
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
//	string:<text>         -> Character String (ASCII)
//	octet:<hex>           -> Octet String (hex bytes, e.g. octet:0a1b2c)
//	oid:<type>:<instance> -> Object Identifier
func parseWriteValue(s string) (bacencoding.ApplicationValue, error) {
	kind, rest, ok := strings.Cut(s, ":")
	if !ok {
		return nil, fmt.Errorf("invalid value %q: expected <type>:<value> (e.g. real:21.5, null:)", s)
	}
	kind = strings.ToLower(strings.TrimSpace(kind))

	switch kind {
	case "null":
		return bacencoding.AppNull{}, nil

	case "bool", "boolean":
		b, err := strconv.ParseBool(strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("invalid bool %q: %w", rest, err)
		}
		return bacencoding.AppBoolean(b), nil

	case "unsigned", "uint", "u":
		n, err := strconv.ParseUint(strings.TrimSpace(rest), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid unsigned %q: %w", rest, err)
		}
		return bacencoding.AppUnsignedInteger(uint32(n)), nil

	case "int", "integer", "signed":
		n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", rest, err)
		}
		return bacencoding.AppInteger(int32(n)), nil

	case "real", "float":
		f, err := strconv.ParseFloat(strings.TrimSpace(rest), 32)
		if err != nil {
			return nil, fmt.Errorf("invalid real %q: %w", rest, err)
		}
		return bacencoding.AppReal(float32(f)), nil

	case "double":
		f, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid double %q: %w", rest, err)
		}
		return bacencoding.AppDouble(f), nil

	case "enum", "enumerated":
		n, err := strconv.ParseUint(strings.TrimSpace(rest), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid enum %q: %w", rest, err)
		}
		return bacencoding.AppEnum(uint32(n)), nil

	case "string", "str", "chars":
		return bacencoding.AppCharacterString(rest), nil

	case "octet", "octetstring", "hex":
		clean := strings.ReplaceAll(strings.TrimSpace(rest), " ", "")
		clean = strings.TrimPrefix(clean, "0x")
		b, err := hex.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("invalid octet-string hex %q: %w", rest, err)
		}
		return bacencoding.AppOctetString(b), nil

	case "oid", "objectid", "object-identifier":
		oid, err := parseObjectID(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid object-identifier %q: %w", rest, err)
		}
		return bacencoding.AppObjectIdentifier(oid), nil

	default:
		return nil, fmt.Errorf("unknown value type %q (supported: null bool unsigned int real double enum string octet oid)", kind)
	}
}

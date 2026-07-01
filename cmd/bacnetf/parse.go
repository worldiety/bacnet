package main

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
)

// defaultUDPPort is the standard BACnet/IP UDP port (0xBAC0).
const defaultUDPPort = 47808

// deviceRef is a parsed <device> command-line argument. Exactly one of the two
// forms is populated:
//
//   - IsID == true: the operator gave a BACnet device instance (Instance). Its
//     transport address must be resolved via a scoped Who-Is before use.
//   - IsID == false: the operator gave a BACnet/IP transport address (Address),
//     usable directly.
type deviceRef struct {
	IsID     bool
	Instance uint32
	Address  netprim.Address
}

// parseDeviceRef parses a <device> argument, auto-detecting whether it is a
// BACnet device ID or a BACnet/IP transport address.
//
// Accepted forms:
//
//	10.6.6.123          -> IP transport address, port 47808
//	10.6.6.123:47809    -> IP transport address, explicit port
//	5123                -> device ID (instance 5123)
//	device:5123         -> device ID (explicit)
//	#5123               -> device ID (explicit)
//
// Disambiguation: any value containing a dot is treated as an IPv4 address;
// otherwise it is treated as a device ID.
func parseDeviceRef(s string) (deviceRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return deviceRef{}, fmt.Errorf("empty device")
	}

	// Explicit device-ID forms.
	if rest, ok := strings.CutPrefix(s, "#"); ok {
		return parseDeviceIDRef(rest)
	}
	if rest, ok := cutPrefixFold(s, "device:"); ok {
		return parseDeviceIDRef(rest)
	}

	// A dot means an IPv4 transport address; otherwise a bare device ID.
	if strings.Contains(s, ".") {
		addr, err := parseDeviceAddr(s)
		if err != nil {
			return deviceRef{}, err
		}
		return deviceRef{Address: addr}, nil
	}

	return parseDeviceIDRef(s)
}

// parseDeviceIDRef parses a bare device instance number into a deviceRef.
func parseDeviceIDRef(s string) (deviceRef, error) {
	s = strings.TrimSpace(s)
	inst, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return deviceRef{}, fmt.Errorf("invalid device id %q: expected a number, device:N, #N, or an IP address", s)
	}
	di, err := types.NewDeviceInstance(uint32(inst))
	if err != nil {
		return deviceRef{}, fmt.Errorf("invalid device id %q: %w", s, err)
	}
	return deviceRef{IsID: true, Instance: uint32(di)}, nil
}

// cutPrefixFold is strings.CutPrefix with case-insensitive prefix matching.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

// parseDeviceAddr parses a BACnet/IP transport address given on the command line.
//
// Accepted forms:
//
//	10.6.6.123          -> 10.6.6.123:47808 on the local network
//	10.6.6.123:47809    -> explicit port on the local network
//
// Only BACnet/IP (IPv4) unicast addresses on the local network are supported;
// this matches how the library targets confirmed requests.
func parseDeviceAddr(s string) (netprim.Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netprim.Address{}, fmt.Errorf("empty device address")
	}

	// Bare IPv4 without a port: append the default BACnet port.
	if !strings.Contains(s, ":") {
		if _, err := netip.ParseAddr(s); err != nil {
			return netprim.Address{}, fmt.Errorf("invalid device address %q: %w", s, err)
		}
		s = fmt.Sprintf("%s:%d", s, defaultUDPPort)
	}

	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return netprim.Address{}, fmt.Errorf("invalid device address %q: %w", s, err)
	}
	if !ap.Addr().Is4() {
		return netprim.Address{}, fmt.Errorf("device address %q must be IPv4 (BACnet/IP)", s)
	}

	return netprim.NewAddressFromAddrPort(ap), nil
}

// parseObjectID parses an object identifier in the form "type:instance".
//
// The type may be a known name (e.g. "analog-value", "device") or a raw
// numeric object-type value (e.g. "2"). The instance is an unsigned integer.
//
//	analog-value:270
//	device:1234
//	2:270
func parseObjectID(s string) (types.ObjectIdentifier, error) {
	s = strings.TrimSpace(s)
	typePart, instPart, ok := strings.Cut(s, ":")
	if !ok {
		return 0, fmt.Errorf("invalid object %q: expected <type>:<instance> (e.g. analog-value:1)", s)
	}

	ot, err := parseObjectType(typePart)
	if err != nil {
		return 0, err
	}

	inst, err := strconv.ParseUint(strings.TrimSpace(instPart), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid object instance %q: %w", instPart, err)
	}

	oid, err := types.NewObjectIdentifier(ot, uint32(inst))
	if err != nil {
		return 0, fmt.Errorf("invalid object %q: %w", s, err)
	}
	return oid, nil
}

// parseObjectType resolves an object type from a name or a numeric value.
func parseObjectType(s string) (types.ObjectType, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty object type")
	}

	if ot, ok := objectTypeByName[s]; ok {
		return ot, nil
	}

	// Numeric fallback.
	if n, err := strconv.ParseUint(s, 10, 16); err == nil {
		ot := types.ObjectType(n)
		if !ot.Valid() {
			return 0, fmt.Errorf("object type %d out of range (max %d)", n, types.ObjectTypeMax)
		}
		return ot, nil
	}

	return 0, fmt.Errorf("unknown object type %q (try a name like analog-input or a number)", s)
}

// parsePropertyID resolves a property identifier from a name or numeric value.
//
//	present-value
//	object-name
//	85
func parsePropertyID(s string) (types.PropertyIdentifier, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty property identifier")
	}

	if pid, ok := propertyByName[s]; ok {
		return pid, nil
	}

	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		return types.PropertyIdentifier(n), nil
	}

	return 0, fmt.Errorf("unknown property %q (try a name like present-value or a number)", s)
}

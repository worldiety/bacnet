package bip

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"go.wdy.de/bacnet"
)

// ip4MACLen is the length of a BACnet/IP MAC address (4-byte IPv4 + 2-byte port).
const ip4MACLen = 6

// AddrPortToAddress converts a BACnet/IP UDP address into a BACnet local-network address.
//
// Per Annex J of ANSI/ASHRAE 135-2024, each BACnet/IP node is identified by a
// 6-byte B/IP address consisting of the IPv4 address (4 bytes, big-endian) followed
// by the UDP port (2 bytes, big-endian). The returned Address has Network set to
// bacnet.LocalNetwork (0).
//
// addr must be a valid IPv4 address-port pair; IPv6 addresses are not supported.
func AddrPortToAddress(addr netip.AddrPort) (bacnet.Address, error) {
	if !addr.IsValid() || !addr.Addr().Is4() {
		return bacnet.Address{}, bacnet.NewValidationError("addr", addr, ErrInvalidIPAddress)
	}

	ip4 := addr.Addr().As4()
	mac := make([]byte, ip4MACLen)
	copy(mac[:4], ip4[:])
	binary.BigEndian.PutUint16(mac[4:], addr.Port())

	return bacnet.Address{
		Network: bacnet.LocalNetwork,
		MAC:     mac,
	}, nil
}

// AddressToAddrPort converts a BACnet local-network address back to a UDP address-port.
//
// The address must be a local-network address (Network == bacnet.LocalNetwork) whose
// MAC field holds exactly 6 bytes encoded as [ip0, ip1, ip2, ip3, port_hi, port_lo]
// per Annex J of ANSI/ASHRAE 135-2024.
//
// Returns ErrUnsupportedAddress if the address is not convertible.
func AddressToAddrPort(addr bacnet.Address) (netip.AddrPort, error) {
	if !addr.Network.IsLocal() {
		return netip.AddrPort{}, fmt.Errorf("%w: non-local network %d", ErrUnsupportedAddress, addr.Network)
	}

	if len(addr.MAC) != ip4MACLen {
		return netip.AddrPort{}, fmt.Errorf("%w: MAC must be %d bytes for BACnet/IP, got %d", ErrUnsupportedAddress, ip4MACLen, len(addr.MAC))
	}

	ip := netip.AddrFrom4([4]byte{addr.MAC[0], addr.MAC[1], addr.MAC[2], addr.MAC[3]})
	port := binary.BigEndian.Uint16(addr.MAC[4:6])

	return netip.AddrPortFrom(ip, port), nil
}

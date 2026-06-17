package netprim

import (
	"encoding/binary"
	"net/netip"
	"slices"

	"go.wdy.de/bacnet/common/errors"
)

// Address identifies a BACnet station on a local or remote network.
type Address struct {
	Network  NetworkNumber
	AddrPort netip.AddrPort
}

func NewAddressFromAddrPort(addrPort netip.AddrPort) Address {
	return Address{
		Network:  LocalNetwork,
		AddrPort: addrPort,
	}
}

// NewAddress constructs an address and defensively copies the MAC bytes.
func NewAddress(network NetworkNumber, addPort []byte) (Address, error) {
	if len(addPort) != 6 {
		return Address{}, errors.NewValidationError("AddrPort", len(addPort), errors.ErrInvalidBacnetAddress)
	}

	cloned := make([]byte, len(addPort))
	copy(cloned, addPort)

	port := binary.BigEndian.Uint16(addPort[4:6])
	addr := netip.AddrFrom4([4]byte{addPort[0], addPort[1], addPort[2], addPort[3]})

	return Address{
		Network:  network,
		AddrPort: netip.AddrPortFrom(addr, port),
	}, nil
}

// IsLocal reports whether the address belongs to the local BACnet network.
func (a Address) IsLocal() bool {
	return a.Network.IsLocal()
}

// AddrPortBytes returns a defensive copy of the AddrPort address bytes.
func (a Address) AddrPortBytes() []byte {
	bytes := make([]byte, bacNetAddrLen)

	copy(bytes[0:4], a.AddrPort.Addr().AsSlice())
	binary.BigEndian.PutUint16(bytes[4:6], a.AddrPort.Port())

	return bytes
}

func (a Address) Equal(b Address) bool {
	return a.Network == b.Network && slices.Equal(a.AddrPort.Addr().AsSlice(), b.AddrPort.Addr().AsSlice()) && a.AddrPort.Port() == b.AddrPort.Port()
}

// AddrPortToAddress converts a BACnet/IP UDP address into a BACnet local-network address.
//
// Per Annex J of ANSI/ASHRAE 135-2024, each BACnet/IP node is identified by a
// 6-byte B/IP address consisting of the IPv4 address (4 bytes, big-endian) followed
// by the UDP port (2 bytes, big-endian). The returned Address has Network set to
// bacnet.LocalNetwork (0).
//
// addr must be a valid IPv4 address-port pair; IPv6 addresses are not supported.
func AddrPortToAddress(addr netip.AddrPort) (Address, error) {
	if !addr.IsValid() || !addr.Addr().Is4() {
		return Address{}, errors.NewValidationError("addr", addr, errors.ErrInvalidIPAddress)
	}

	return Address{
		Network:  LocalNetwork,
		AddrPort: addr,
	}, nil
}

// bacNetAddrLen is the length of a BACnet/IP MAC address (4-byte IPv4 + 2-byte port).
const bacNetAddrLen = 6

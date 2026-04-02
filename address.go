package bacnet

// Address identifies a BACnet station on a local or remote network.
type Address struct {
	Network NetworkNumber
	MAC     []byte
}

// NewAddress constructs an address and defensively copies the MAC bytes.
func NewAddress(network NetworkNumber, mac []byte) (Address, error) {
	if len(mac) > 255 {
		return Address{}, NewValidationError("mac", len(mac), ErrInvalidMACAddress)
	}

	cloned := make([]byte, len(mac))
	copy(cloned, mac)

	return Address{
		Network: network,
		MAC:     cloned,
	}, nil
}

// IsLocal reports whether the address belongs to the local BACnet network.
func (a Address) IsLocal() bool {
	return a.Network.IsLocal()
}

// MACBytes returns a defensive copy of the MAC address bytes.
func (a Address) MACBytes() []byte {
	cloned := make([]byte, len(a.MAC))
	copy(cloned, a.MAC)
	return cloned
}

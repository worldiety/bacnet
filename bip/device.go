package bip

import (
	"fmt"
	"net/netip"

	"go.wdy.de/bacnet"
)

// DeviceIp4 defines the interface for a BACnet/IP device (non-BBMD).
//
// A BACnet/IP device can broadcast messages on its local subnet and optionally
// register with a remote BBMD to participate in inter-subnet communication as
// a foreign device (Annex J §J.5).
type DeviceIp4 interface {
	// SendLocalBroadcast encodes msg and delivers it to every BACnet node on
	// the device's local IP subnet via the IPv4 limited-broadcast address
	// (255.255.255.255) on the standard BACnet/IP UDP port.
	SendLocalBroadcast(msg OriginalBroadcastNpdu) error

	// RegisterAsForeignDevice sends a Register-Foreign-Device (0x05) request
	// to the BBMD at bbmdAddr and waits for its BVLCResult response.
	// Returns ErrRegistrationRejected if the BBMD responds with a NAK result code.
	// bbmdAddr must be a valid IPv4 address; the standard BACnet/IP port is used.
	RegisterAsForeignDevice(bbmdAddr netip.Addr) error
}

// NewDeviceIp4 returns a DeviceIp4 backed by conn.
//
// conn must already be bound to the device's local UDP address and port
// (e.g. via NewDatagramConn). ttl is the time-to-live in seconds sent in
// Register-Foreign-Device requests; it must be non-zero.
func NewDeviceIp4(conn DatagramConn, ttl TTL) (DeviceIp4, error) {
	if conn == nil {
		return nil, ErrNilDatagramConn
	}
	if ttl == 0 {
		return nil, bacnet.NewValidationError("ttl", ttl, ErrInvalidTTL)
	}
	return &deviceImpl{conn: conn, ttl: ttl}, nil
}

// deviceImpl is the default BACnet/IP device implementation (non-BBMD).
type deviceImpl struct {
	conn DatagramConn
	ttl  TTL // time-to-live for foreign device registration
}

// SendLocalBroadcast implements DeviceIp4.
func (d *deviceImpl) SendLocalBroadcast(msg OriginalBroadcastNpdu) error {
	raw, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encode original-broadcast-npdu: %w", err)
	}

	dst := netip.AddrPortFrom(netip.MustParseAddr("255.255.255.255"), bacnet.IpDefaultUdpPort)
	if _, err := d.conn.WriteToUDPAddrPort(raw, dst); err != nil {
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}
	return nil
}

// RegisterAsForeignDevice implements DeviceIp4.
func (d *deviceImpl) RegisterAsForeignDevice(bbmdAddr netip.Addr) error {
	if !bbmdAddr.IsValid() || !bbmdAddr.Is4() {
		return bacnet.NewValidationError("bbmd address", bbmdAddr, ErrInvalidIPAddress)
	}

	req, err := NewRegisterForeignDevice(d.ttl)
	if err != nil {
		return fmt.Errorf("build register-foreign-device: %w", err)
	}

	raw, err := req.Encode()
	if err != nil {
		return fmt.Errorf("encode register-foreign-device: %w", err)
	}

	dst := netip.AddrPortFrom(bbmdAddr, bacnet.IpDefaultUdpPort)
	if _, err := d.conn.WriteToUDPAddrPort(raw, dst); err != nil {
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}

	// Read the BVLCResult response from the BBMD.
	buf := make([]byte, DefaultMaxDatagramSize)
	n, _, err := d.conn.ReadFromUDPAddrPort(buf)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReadFailure, err)
	}

	var result BVLCResult
	if err := result.Decode(buf[:n]); err != nil {
		return fmt.Errorf("decode bvlc-result from bbmd: %w", err)
	}

	if result.ResultCode() != ResultCodeSuccessfulCompletion {
		return fmt.Errorf("%w: %v", ErrRegistrationRejected, result.ResultCode())
	}
	return nil
}

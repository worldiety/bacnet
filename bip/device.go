package bip

import (
	"fmt"
	"net/netip"
	"time"

	"go.wdy.de/bacnet/common/errors"
	"go.wdy.de/bacnet/common/log"
	"go.wdy.de/bacnet/common/netprim"
)

const defaultForeignDeviceRegistrationResponseTimeout = 5 * time.Second

type readDeadlineSetter interface {
	SetReadDeadline(t time.Time) error
}

// DeviceIp4 defines the interface for a BACnet/IP device (non-BBMD).
//
// A BACnet/IP device can broadcast messages on its local subnet, send unicast
// messages to a specific peer, and optionally register with a remote BBMD to
// participate in inter-subnet communication as a foreign device (Annex J §J.5).
type DeviceIp4 interface {
	// SendLocalBroadcast encodes msg and delivers it to every BACnet node on
	// the device's local IP subnet via the IPv4 limited-broadcast address
	// (255.255.255.255) on the standard BACnet/IP UDP port.
	SendLocalBroadcast(msg OriginalBroadcastNpdu) error

	// SendUnicast encodes msg and delivers it to the single peer identified by dst.
	// dst must be a valid IPv4 address-port pair.
	SendUnicast(dst netip.AddrPort, msg OriginalUnicastNpdu) error

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
		return nil, errors.NewValidationError("ttl", ttl, ErrInvalidTTL)
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
		log.Logger.Error("bip device encode local broadcast", "error", err)
		return fmt.Errorf("encode original-broadcast-npdu: %w", err)
	}

	dst := netip.AddrPortFrom(netip.MustParseAddr("255.255.255.255"), netprim.IpDefaultUdpPort)
	if _, err := d.conn.WriteToUDPAddrPort(raw, dst); err != nil {
		log.Logger.Error("bip device write local broadcast", "error", err, "dst", dst, "bytes", len(raw))
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}
	return nil
}

// SendUnicast implements DeviceIp4.
// dst must be a valid IPv4 address-port pair; it is used directly as the UDP destination.
func (d *deviceImpl) SendUnicast(dst netip.AddrPort, msg OriginalUnicastNpdu) error {
	if !dst.IsValid() || !dst.Addr().Is4() {
		return errors.NewValidationError("dst", dst, errors.ErrInvalidIPAddress)
	}

	raw, err := msg.Encode()
	if err != nil {
		log.Logger.Error("bip device encode unicast", "error", err, "dst", dst)
		return fmt.Errorf("encode original-unicast-npdu: %w", err)
	}

	if _, err := d.conn.WriteToUDPAddrPort(raw, dst); err != nil {
		log.Logger.Error("bip device write unicast", "error", err, "dst", dst, "bytes", len(raw))
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}
	return nil
}

// RegisterAsForeignDevice implements DeviceIp4.
func (d *deviceImpl) RegisterAsForeignDevice(bbmdAddr netip.Addr) error {
	if !bbmdAddr.IsValid() || !bbmdAddr.Is4() {
		return errors.NewValidationError("bbmd address", bbmdAddr, errors.ErrInvalidIPAddress)
	}

	req, err := NewRegisterForeignDevice(d.ttl)
	if err != nil {
		log.Logger.Error("bip device build register foreign device", "error", err, "ttl", d.ttl)
		return fmt.Errorf("build register-foreign-device: %w", err)
	}

	raw, err := req.Encode()
	if err != nil {
		log.Logger.Error("bip device encode register foreign device", "error", err)
		return fmt.Errorf("encode register-foreign-device: %w", err)
	}

	dst := netip.AddrPortFrom(bbmdAddr, netprim.IpDefaultUdpPort)
	if _, err := d.conn.WriteToUDPAddrPort(raw, dst); err != nil {
		log.Logger.Error("bip device write register foreign device", "error", err, "dst", dst, "bytes", len(raw))
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}

	readDeadline, supportsReadDeadline := d.conn.(readDeadlineSetter)
	if supportsReadDeadline {
		if err := readDeadline.SetReadDeadline(time.Now().Add(defaultForeignDeviceRegistrationResponseTimeout)); err != nil {
			log.Logger.Error("bip device set read deadline", "error", err, "timeout", defaultForeignDeviceRegistrationResponseTimeout)
			return fmt.Errorf("%w: %v", ErrReadFailure, err)
		}
		defer func() {
			_ = readDeadline.SetReadDeadline(time.Time{})
		}()
	}

	// Read the BVLCResult response from the requested BBMD.
	buf := make([]byte, DefaultMaxDatagramSize)
	for {
		n, src, err := d.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			log.Logger.Error("bip device read register response", "error", err)
			return fmt.Errorf("%w: %v", ErrReadFailure, err)
		}

		if src != dst {
			if !supportsReadDeadline {
				log.Logger.Error("bip device unexpected register response source", "error", ErrReadFailure, "src", src, "expected", dst)
				return fmt.Errorf("%w: unexpected response source %v", ErrReadFailure, src)
			}
			continue
		}

		var result BVLCResult
		if err := result.Decode(buf[:n]); err != nil {
			if !supportsReadDeadline {
				log.Logger.Error("bip device decode register response", "error", err, "bytes", n)
				return fmt.Errorf("%w: decode bvlc-result from bbmd: %v", ErrReadFailure, err)
			}
			continue
		}

		if result.ResultCode() != ResultCodeSuccessfulCompletion {
			log.Logger.Error("bip device registration rejected", "error", ErrRegistrationRejected, "result_code", result.ResultCode())
			return fmt.Errorf("%w: %v", ErrRegistrationRejected, result.ResultCode())
		}

		return nil
	}
}

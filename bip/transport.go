package bip

import (
	"fmt"
	"net"
	"net/netip"

	"go.wdy.de/bacnet"
)

const (
	// DefaultMaxDatagramSize is a conservative IPv4 UDP payload budget.
	DefaultMaxDatagramSize = 1476
)

// DatagramConn is the small UDP surface required by the BVLC transport.
//
// The concrete connection is expected to already be bound to the desired local
// UDP address/port before it is passed to NewTransport.
type DatagramConn interface {
	// ReadFromUDPAddrPort reads from the already-bound local socket and returns
	// the sender's remote address/port.
	ReadFromUDPAddrPort(p []byte) (n int, addr netip.AddrPort, err error)
	// WriteToUDPAddrPort sends one datagram to the given remote address/port.
	WriteToUDPAddrPort(p []byte, addr netip.AddrPort) (n int, err error)

	Close() error
}

func NewDatagramConn(addr netip.Addr) (DatagramConn, error) {
	if !addr.IsValid() {
		return nil, fmt.Errorf("invalid ip address: %v", addr)
	}

	if addr.Is4() {
		udpAddr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(addr, BACnetIpDefaultUdpPort))
		conn, err := net.ListenUDP("udp4", udpAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to listen on %v: %w", udpAddr, err)
		}
		return conn, nil
	}

	if addr.Is6() {
		udpAddr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(addr, BACnetIpDefaultUdpPort))
		conn, err := net.ListenUDP("udp6", udpAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to listen on %v: %w", udpAddr, err)
		}

		return conn, nil
	}

	return nil, fmt.Errorf("invalid ip address: %v", addr)
}

// Transport sends and receives BVLC frames via UDP-like datagrams.
type Transport struct {
	conn            DatagramConn
	maxDatagramSize int
}

func (t *Transport) Close() error {
	return t.conn.Close()
}

// NewTransport validates and constructs a BVLC transport.
func NewTransport(conn DatagramConn, maxDatagramSize int) (*Transport, error) {
	if conn == nil {
		return nil, ErrNilDatagramConn
	}
	if maxDatagramSize < BVLCHeaderLen {
		return nil, &bacnet.ValidationError{Field: "max datagram size", Value: maxDatagramSize, Err: ErrInvalidLength}
	}

	return &Transport{conn: conn, maxDatagramSize: maxDatagramSize}, nil
}

// ReceiveFrame reads one datagram and decodes it as BVLC frame.
//
// The returned netip.AddrPort is the remote sender address/port.
func (t *Transport) ReceiveFrame() (Frame, netip.AddrPort, error) {
	buf := make([]byte, t.maxDatagramSize)
	n, addr, err := t.conn.ReadFromUDPAddrPort(buf)
	if err != nil {
		return Frame{}, netip.AddrPort{}, fmt.Errorf("%w: %v", ErrReadFailure, err)
	}

	frame, err := DecodeFrame(buf[:n])
	if err != nil {
		return Frame{}, netip.AddrPort{}, err
	}
	return frame, addr, nil
}

// SendFrame encodes and writes one BVLC frame datagram.
func (t *Transport) SendFrame(addr netip.AddrPort, frame Frame) error {
	raw, err := frame.Encode()
	if err != nil {
		return err
	}
	if len(raw) > t.maxDatagramSize {
		return &bacnet.ValidationError{Field: "datagram length", Value: len(raw), Err: ErrDatagramTooLarge}
	}

	if _, err := t.conn.WriteToUDPAddrPort(raw, addr); err != nil {
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}
	return nil
}

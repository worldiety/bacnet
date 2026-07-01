package bip

import (
	"errors"
	"net/netip"
	"slices"
	"testing"

	bacneterrors "github.com/worldiety/bacnet/common/errors"
)

type fakeDatagramConn struct {
	readData []byte
	readAddr netip.AddrPort
	readErr  error
	writeErr error

	writtenData []byte
	writtenAddr netip.AddrPort
}

func (f *fakeDatagramConn) Close() error {
	return nil
}

func (f *fakeDatagramConn) ReadFromUDPAddrPort(p []byte) (int, netip.AddrPort, error) {
	if f.readErr != nil {
		return 0, netip.AddrPort{}, f.readErr
	}
	copy(p, f.readData)
	return len(f.readData), f.readAddr, nil
}

func (f *fakeDatagramConn) WriteToUDPAddrPort(p []byte, addr netip.AddrPort) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.writtenData = slices.Clone(p)
	f.writtenAddr = addr
	return len(p), nil
}

func TestNewTransportValidation(t *testing.T) {
	tests := []struct {
		name    string
		conn    DatagramConn
		size    int
		wantErr error
	}{
		{name: "valid", conn: &fakeDatagramConn{}, size: DefaultMaxDatagramSize},
		{name: "nil conn", conn: nil, size: DefaultMaxDatagramSize, wantErr: ErrNilDatagramConn},
		{name: "invalid size", conn: &fakeDatagramConn{}, size: 3, wantErr: ErrInvalidLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTransport(tt.conn, tt.size)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("NewTransport error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewTransport error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTransportSendReceiveFrame(t *testing.T) {
	tests := []struct {
		name string
		addr netip.AddrPort
	}{
		{name: "ipv4", addr: netip.MustParseAddrPort("192.168.1.10:47808")},
		{name: "ipv6", addr: netip.MustParseAddrPort("[2001:db8::10]:47808")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := NewFrameForAddress(tt.addr.Addr(), FunctionOriginalBroadcastNPDU, []byte{0x01, 0x02})
			if err != nil {
				t.Fatalf("NewFrameForAddress returned error: %v", err)
			}
			raw, err := frame.Encode()
			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}

			conn := &fakeDatagramConn{readData: raw, readAddr: tt.addr}
			transport, err := NewTransport(conn, DefaultMaxDatagramSize)
			if err != nil {
				t.Fatalf("NewTransport returned error: %v", err)
			}

			if err := transport.SendFrame(tt.addr, frame); err != nil {
				t.Fatalf("SendFrame returned error: %v", err)
			}
			if conn.writtenAddr != tt.addr {
				t.Fatalf("written address = %v, want %v", conn.writtenAddr, tt.addr)
			}
			if len(conn.writtenData) != len(raw) {
				t.Fatalf("written data len = %d, want %d", len(conn.writtenData), len(raw))
			}

			decoded, gotAddr, err := transport.ReceiveFrame()
			if err != nil {
				t.Fatalf("ReceiveFrame returned error: %v", err)
			}
			if gotAddr != tt.addr {
				t.Fatalf("received address = %v, want %v", gotAddr, tt.addr)
			}
			if decoded.Type != frame.Type {
				t.Fatalf("decoded type = %v, want %v", decoded.Type, frame.Type)
			}
			if decoded.Function != FunctionOriginalBroadcastNPDU {
				t.Fatalf("decoded function = %v, want %v", decoded.Function, FunctionOriginalBroadcastNPDU)
			}
		})
	}
}

func TestAddressFamilyHelpers(t *testing.T) {
	tests := []struct {
		name        string
		addr        netip.Addr
		wantNetwork string
		wantType    BVLCType
		wantErr     error
	}{
		{name: "ipv4", addr: netip.MustParseAddr("10.0.0.20"), wantNetwork: "udp4", wantType: BVLCTypeBACnetIP},
		{name: "ipv6", addr: netip.MustParseAddr("2001:db8::20"), wantNetwork: "udp6", wantType: BVLCTypeBACnetIP6},
		{name: "invalid", addr: netip.Addr{}, wantErr: bacneterrors.ErrInvalidIPAddress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network, err := udpNetworkForAddress(tt.addr)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("udpNetworkForAddress error = %v, want %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("udpNetworkForAddress error = %v", err)
				}
				if network != tt.wantNetwork {
					t.Fatalf("network = %q, want %q", network, tt.wantNetwork)
				}
			}

			frameType, err := bvlcTypeForAddress(tt.addr)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("bvlcTypeForAddress error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("bvlcTypeForAddress error = %v", err)
			}
			if frameType != tt.wantType {
				t.Fatalf("type = %v, want %v", frameType, tt.wantType)
			}
		})
	}
}

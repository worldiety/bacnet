package bip

import (
	"errors"
	"net/netip"
	"testing"
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
	f.writtenData = cloneBytes(p)
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
	frame, err := NewFrame(FunctionOriginalBroadcastNPDU, []byte{0x01, 0x02})
	if err != nil {
		t.Fatalf("NewFrame returned error: %v", err)
	}
	raw, err := frame.Encode()
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	addr := netip.MustParseAddrPort("192.168.1.10:47808")
	conn := &fakeDatagramConn{readData: raw, readAddr: addr}

	transport, err := NewTransport(conn, DefaultMaxDatagramSize)
	if err != nil {
		t.Fatalf("NewTransport returned error: %v", err)
	}

	if err := transport.SendFrame(addr, frame); err != nil {
		t.Fatalf("SendFrame returned error: %v", err)
	}
	if conn.writtenAddr != addr {
		t.Fatalf("written address = %v, want %v", conn.writtenAddr, addr)
	}
	if len(conn.writtenData) != len(raw) {
		t.Fatalf("written data len = %d, want %d", len(conn.writtenData), len(raw))
	}

	decoded, gotAddr, err := transport.ReceiveFrame()
	if err != nil {
		t.Fatalf("ReceiveFrame returned error: %v", err)
	}
	if gotAddr != addr {
		t.Fatalf("received address = %v, want %v", gotAddr, addr)
	}
	if decoded.Function != FunctionOriginalBroadcastNPDU {
		t.Fatalf("decoded function = %v, want %v", decoded.Function, FunctionOriginalBroadcastNPDU)
	}
}

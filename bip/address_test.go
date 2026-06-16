package bip

import (
	"errors"
	"net/netip"
	"testing"

	"go.wdy.de/bacnet/common/netprim"
)

// TestAddrPortToAddress verifies round-trip and error cases.
func TestAddrPortToAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   netip.AddrPort
		wantMAC []byte
		wantErr error
	}{
		{
			name:    "valid ipv4",
			input:   netip.MustParseAddrPort("192.168.1.10:47808"),
			wantMAC: []byte{192, 168, 1, 10, 0xBA, 0xC0},
		},
		{
			name:    "valid ipv4 port 1234",
			input:   netip.MustParseAddrPort("10.0.0.1:1234"),
			wantMAC: []byte{10, 0, 0, 1, 0x04, 0xD2},
		},
		{
			name:    "zero value",
			input:   netip.AddrPort{},
			wantErr: ErrInvalidIPAddress,
		},
		{
			name:    "ipv6 rejected",
			input:   netip.MustParseAddrPort("[::1]:47808"),
			wantErr: ErrInvalidIPAddress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddrPortToAddress(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Network != netprim.LocalNetwork {
				t.Errorf("Network = %d, want %d", got.Network, netprim.LocalNetwork)
			}
			if len(got.MAC) != len(tt.wantMAC) {
				t.Fatalf("MAC len = %d, want %d", len(got.MAC), len(tt.wantMAC))
			}
			for i := range tt.wantMAC {
				if got.MAC[i] != tt.wantMAC[i] {
					t.Errorf("MAC[%d] = 0x%02x, want 0x%02x", i, got.MAC[i], tt.wantMAC[i])
				}
			}
		})
	}
}

// TestAddressToAddrPort verifies round-trip and error cases.
func TestAddressToAddrPort(t *testing.T) {
	tests := []struct {
		name     string
		input    netprim.Address
		wantAddr netip.AddrPort
		wantErr  error
	}{
		{
			name:     "valid 6-byte local",
			input:    netprim.Address{Network: netprim.LocalNetwork, MAC: []byte{192, 168, 1, 10, 0xBA, 0xC0}},
			wantAddr: netip.MustParseAddrPort("192.168.1.10:47808"),
		},
		{
			name:    "non-local network rejected",
			input:   netprim.Address{Network: 100, MAC: []byte{192, 168, 1, 10, 0xBA, 0xC0}},
			wantErr: ErrUnsupportedAddress,
		},
		{
			name:    "4-byte MAC rejected",
			input:   netprim.Address{Network: netprim.LocalNetwork, MAC: []byte{192, 168, 1, 10}},
			wantErr: ErrUnsupportedAddress,
		},
		{
			name:    "nil MAC rejected",
			input:   netprim.Address{Network: netprim.LocalNetwork, MAC: nil},
			wantErr: ErrUnsupportedAddress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddressToAddrPort(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantAddr {
				t.Errorf("AddrPort = %v, want %v", got, tt.wantAddr)
			}
		})
	}
}

// TestAddrPortRoundTrip verifies that AddrPortToAddress → AddressToAddrPort is lossless.
func TestAddrPortRoundTrip(t *testing.T) {
	inputs := []netip.AddrPort{
		netip.MustParseAddrPort("192.168.1.10:47808"),
		netip.MustParseAddrPort("10.0.0.1:1234"),
		netip.MustParseAddrPort("172.16.0.255:65535"),
	}
	for _, in := range inputs {
		t.Run(in.String(), func(t *testing.T) {
			addr, err := AddrPortToAddress(in)
			if err != nil {
				t.Fatalf("AddrPortToAddress: %v", err)
			}
			got, err := AddressToAddrPort(addr)
			if err != nil {
				t.Fatalf("AddressToAddrPort: %v", err)
			}
			if got != in {
				t.Errorf("round-trip: got %v, want %v", got, in)
			}
		})
	}
}

// TestAddrPortToAddressCopiesMAC ensures the returned MAC is a fresh slice,
// not a reference into the AddrPort value.
func TestAddrPortToAddressCopiesMAC(t *testing.T) {
	addr, err := AddrPortToAddress(netip.MustParseAddrPort("1.2.3.4:47808"))
	if err != nil {
		t.Fatalf("AddrPortToAddress: %v", err)
	}
	orig := make([]byte, len(addr.MAC))
	copy(orig, addr.MAC)
	addr.MAC[0] = 0xFF
	got, err := AddressToAddrPort(netprim.Address{Network: netprim.LocalNetwork, MAC: orig})
	if err != nil {
		t.Fatalf("AddressToAddrPort: %v", err)
	}
	if got.Addr().As4()[0] != 1 {
		t.Errorf("mutation leaked: got %v, expected first octet 1", got)
	}
}

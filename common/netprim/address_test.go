package netprim

import (
	"errors"
	"net/netip"
	"testing"

	bacneterrors "go.wdy.de/bacnet/common/errors"
)

func TestNewAddressRejectsOversizedMAC(t *testing.T) {
	mac := make([]byte, 256)
	if _, err := NewAddress(LocalNetwork, mac); err == nil {
		t.Fatal("expected oversized MAC address error")
	}
}

// TestAddrPortToAddress verifies round-trip and error cases.
func TestAddrPortToAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    netip.AddrPort
		wantAddr Address
		wantErr  error
	}{
		{
			name:  "valid ipv4",
			input: netip.MustParseAddrPort("192.168.1.10:47808"),
			wantAddr: Address{
				Network:  0,
				AddrPort: netip.MustParseAddrPort("192.168.1.10:47808"),
			},
		},
		{
			name:  "valid ipv4 port 1234",
			input: netip.MustParseAddrPort("10.0.0.1:1234"),
			wantAddr: Address{
				Network:  0,
				AddrPort: netip.MustParseAddrPort("10.0.0.1:1234"),
			},
		},
		{
			name:    "zero value",
			input:   netip.AddrPort{},
			wantErr: bacneterrors.ErrInvalidIPAddress,
		},
		{
			name:    "ipv6 rejected",
			input:   netip.MustParseAddrPort("[::1]:47808"),
			wantErr: bacneterrors.ErrInvalidIPAddress,
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

			if !got.Equal(tt.wantAddr) {
				t.Fatalf("Addr = %v want %v", got, tt.wantAddr)
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
			got := addr.AddrPort
			if got != in {
				t.Errorf("round-trip: got %v, want %v", got, in)
			}
		})
	}
}

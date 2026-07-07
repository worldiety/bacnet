package netprim

import (
	"errors"
	"net/netip"
	"testing"

	bacneterrors "github.com/worldiety/bacnet/common/errors"
)

func TestNewAddressRejectsOversizedMAC(t *testing.T) {
	mac := make([]byte, 256)
	if _, err := NewAddress(LocalNetwork, mac); err == nil {
		t.Fatal("expected oversized MAC address error")
	}
}

func TestRoutedAddress(t *testing.T) {
	router := netip.MustParseAddrPort("10.6.6.110:47808")
	a := NewRoutedAddress(router, 4, []byte{0x22})

	if !a.IsRouted() {
		t.Fatal("IsRouted = false, want true for a routed device")
	}
	if a.IsLocal() {
		t.Fatal("IsLocal = true, want false")
	}
	if a.AddrPort != router {
		t.Fatalf("AddrPort = %v, want router %v", a.AddrPort, router)
	}
	if a.Network != 4 {
		t.Fatalf("Network = %d, want 4", a.Network)
	}
	if len(a.MAC) != 1 || a.MAC[0] != 0x22 {
		t.Fatalf("MAC = % x, want 22", a.MAC)
	}

	// NewRoutedAddress must copy the MAC (no aliasing of caller's slice).
	mac := []byte{0x22}
	b := NewRoutedAddress(router, 4, mac)
	mac[0] = 0x99
	if b.MAC[0] != 0x22 {
		t.Fatal("NewRoutedAddress did not copy the MAC slice")
	}

	// A local BACnet/IP address is not routed.
	local := NewAddressFromAddrPort(router)
	if local.IsRouted() {
		t.Fatal("local address reported as routed")
	}
}

func TestAddressEqualMACAware(t *testing.T) {
	router := netip.MustParseAddrPort("10.6.6.110:47808")
	a := NewRoutedAddress(router, 4, []byte{0x22})
	b := NewRoutedAddress(router, 4, []byte{0x23}) // same router+net, different node
	if a.Equal(b) {
		t.Fatal("routed addresses with different MACs must not be equal")
	}
	c := NewRoutedAddress(router, 4, []byte{0x22})
	if !a.Equal(c) {
		t.Fatal("routed addresses with identical fields must be equal")
	}
	// A local address with the same AddrPort but no MAC must differ.
	if a.Equal(NewAddressFromAddrPort(router)) {
		t.Fatal("routed address must not equal a local address at the same IP")
	}
}

func TestAddressString(t *testing.T) {
	router := netip.MustParseAddrPort("10.6.6.110:47808")
	if got := NewRoutedAddress(router, 4, []byte{0x22}).String(); got != "4:22@10.6.6.110:47808" {
		t.Fatalf("routed String = %q", got)
	}
	if got := NewAddressFromAddrPort(router).String(); got != "10.6.6.110:47808" {
		t.Fatalf("local String = %q", got)
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

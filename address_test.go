package bacnet

import "testing"

func TestNewAddressCopiesMAC(t *testing.T) {
	mac := []byte{0x0a, 0x0b, 0x0c}
	addr, err := NewAddress(LocalNetwork, mac)
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	mac[0] = 0xff
	if got := addr.MAC[0]; got != 0x0a {
		t.Fatalf("address MAC was not copied, got 0x%02x", got)
	}

	copied := addr.MACBytes()
	copied[1] = 0xee
	if got := addr.MAC[1]; got != 0x0b {
		t.Fatalf("MACBytes should return a defensive copy, got 0x%02x", got)
	}

	if !addr.IsLocal() {
		t.Fatal("expected local address")
	}
}

func TestNewAddressRejectsOversizedMAC(t *testing.T) {
	mac := make([]byte, 256)
	if _, err := NewAddress(LocalNetwork, mac); err == nil {
		t.Fatal("expected oversized MAC address error")
	}
}


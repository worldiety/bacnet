package client

import (
	"net/netip"
	"testing"

	"github.com/worldiety/bacnet/apdu"
	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/common/types"
)

func TestDeviceFromIndicationRouted(t *testing.T) {
	router := netip.MustParseAddrPort("10.6.6.110:47808")
	oid, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 2034)

	ind := apdu.IAmIndication{
		Source:                netprim.NewRoutedAddress(router, 4, []byte{0x22}),
		DeviceIdentifier:      oid,
		MaxAPDULengthAccepted: 480,
		VendorID:              39,
	}
	d := deviceFromIndication(ind)

	if !d.IsRouted() {
		t.Fatal("Device.IsRouted() = false, want true")
	}
	if d.Network != 4 {
		t.Fatalf("Network = %d, want 4", d.Network)
	}
	if len(d.MAC) != 1 || d.MAC[0] != 0x22 {
		t.Fatalf("MAC = % x, want 22", d.MAC)
	}
	if d.Address != router {
		t.Fatalf("Address = %v, want router %v", d.Address, router)
	}

	// Target() must preserve the routing so a follow-up request reaches the
	// device through its router.
	tgt := d.Target()
	if tgt.addr.Network != 4 || len(tgt.addr.MAC) != 1 || tgt.addr.MAC[0] != 0x22 {
		t.Fatalf("Target address lost routing: %+v", tgt.addr)
	}
	if tgt.addr.AddrPort != router {
		t.Fatalf("Target AddrPort = %v, want router %v", tgt.addr.AddrPort, router)
	}
}

func TestDeviceFromIndicationLocal(t *testing.T) {
	ip := netip.MustParseAddrPort("10.6.6.1:47808")
	oid, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 1)

	d := deviceFromIndication(apdu.IAmIndication{
		Source:           netprim.NewAddressFromAddrPort(ip),
		DeviceIdentifier: oid,
	})
	if d.IsRouted() {
		t.Fatal("local device reported as routed")
	}
	if len(d.MAC) != 0 {
		t.Fatalf("local device MAC = % x, want empty", d.MAC)
	}
	if tgt := d.Target(); tgt.addr.IsRouted() {
		t.Fatal("local device Target must not be routed")
	}
}

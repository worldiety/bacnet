package apdu

import (
	"net/netip"
	"testing"

	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/npdu"
)

// unconfirmedWhoIsAPDU is a minimal valid outbound APDU for building NPDUs.
func unconfirmedWhoIsAPDU() outboundAPDU {
	return outboundAPDU{
		Type:          PDUTypeUnconfirmedRequest,
		ServiceChoice: ServiceChoiceWhoIs,
		Payload:       nil,
	}
}

// dnetValue dereferences a DNET pointer, failing the test if it is absent.
func dnetValue(t *testing.T, pkt npdu.NetworkLayerProtocolDataUnit) npdu.UltimateDestinationNetworkNumber {
	t.Helper()
	d := pkt.DNET()
	if d == nil {
		t.Fatal("DNET is nil, want a destination network")
	}
	return *d
}

func TestBuildOutboundNPDU(t *testing.T) {
	bcastIP := netprim.NewAddressFromAddrPort(netip.MustParseAddrPort("255.255.255.255:47808"))

	t.Run("local has no destination specifier", func(t *testing.T) {
		pkt, err := buildOutboundNPDU(bcastIP, netprim.NetworkPriorityNormal, false, unconfirmedWhoIsAPDU())
		if err != nil {
			t.Fatalf("buildOutboundNPDU: %v", err)
		}
		if pkt.HasDestinationSpecifier() {
			t.Fatal("local NPDU should not carry a destination specifier")
		}
	})

	t.Run("global broadcast has DNET 0xFFFF and no DADR", func(t *testing.T) {
		dst := netprim.Address{Network: netprim.GlobalBroadcastNetwork, AddrPort: bcastIP.AddrPort}
		pkt, err := buildOutboundNPDU(dst, netprim.NetworkPriorityNormal, false, unconfirmedWhoIsAPDU())
		if err != nil {
			t.Fatalf("buildOutboundNPDU (global): %v", err)
		}
		if !pkt.HasDestinationSpecifier() {
			t.Fatal("global-broadcast NPDU must carry a destination specifier")
		}
		if got := dnetValue(t, pkt); got != npdu.UltimateDestinationNetworkNumber(netprim.GlobalBroadcastNetwork) {
			t.Fatalf("DNET = %#x, want 0xFFFF", uint16(got))
		}
		if dadr := pkt.DADR(); len(dadr) != 0 {
			t.Fatalf("global-broadcast NPDU must have empty DADR (DLEN=0), got % x", dadr)
		}
		// It must round-trip through the wire codec.
		wire, err := pkt.Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var decoded npdu.NetworkLayerProtocolDataUnit
		if err := decoded.Decode(wire); err != nil {
			t.Fatalf("decode round-trip: %v", err)
		}
		if got := dnetValue(t, decoded); got != npdu.UltimateDestinationNetworkNumber(netprim.GlobalBroadcastNetwork) {
			t.Fatalf("decoded DNET = %#x, want 0xFFFF", uint16(got))
		}
	})

	t.Run("specific remote network keeps IP MAC as DADR", func(t *testing.T) {
		dst := netprim.Address{Network: 2001, AddrPort: netip.MustParseAddrPort("10.6.6.110:47808")}
		pkt, err := buildOutboundNPDU(dst, netprim.NetworkPriorityNormal, false, unconfirmedWhoIsAPDU())
		if err != nil {
			t.Fatalf("buildOutboundNPDU (remote): %v", err)
		}
		if !pkt.HasDestinationSpecifier() {
			t.Fatal("remote NPDU must carry a destination specifier")
		}
		if got := dnetValue(t, pkt); got != npdu.UltimateDestinationNetworkNumber(2001) {
			t.Fatalf("DNET = %d, want 2001", uint16(got))
		}
		if dadr := pkt.DADR(); len(dadr) != 6 {
			t.Fatalf("remote NPDU DADR should be the 6-byte IP MAC, got % x", dadr)
		}
	})

	t.Run("routed device uses node MAC as DADR", func(t *testing.T) {
		// An MS/TP node (net 4, MAC 0x22) reached via a router at 10.6.6.110.
		dst := netprim.NewRoutedAddress(netip.MustParseAddrPort("10.6.6.110:47808"), 4, []byte{0x22})
		pkt, err := buildOutboundNPDU(dst, netprim.NetworkPriorityNormal, true, unconfirmedWhoIsAPDU())
		if err != nil {
			t.Fatalf("buildOutboundNPDU (routed): %v", err)
		}
		if got := dnetValue(t, pkt); got != npdu.UltimateDestinationNetworkNumber(4) {
			t.Fatalf("DNET = %d, want 4", uint16(got))
		}
		dadr := pkt.DADR()
		if len(dadr) != 1 || dadr[0] != 0x22 {
			t.Fatalf("routed NPDU DADR should be the 1-byte node MAC, got % x", dadr)
		}
	})
}

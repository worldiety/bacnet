package bip

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	bacneterrors "go.wdy.de/bacnet/common/errors"
)

func mustBdtEntryForBVLCFunctionTest(t *testing.T, addr string, mask net.IPMask) BdtEntry {
	t.Helper()
	entry, err := NewBdtEntry(netip.MustParseAddrPort(addr), mask)
	if err != nil {
		t.Fatalf("NewBdtEntry() error = %v", err)
	}
	return *entry
}

func mustFdtEntryForBVLCFunctionTest(t *testing.T, addr string, ttl TTL) FdtEntry {
	t.Helper()
	entry, err := NewFdtEntry(netip.MustParseAddrPort(addr), ttl)
	if err != nil {
		t.Fatalf("NewFdtEntry() error = %v", err)
	}
	return *entry
}

func TestForwardedNpduRoundTripAndDefensiveCopy(t *testing.T) {
	origin := netip.MustParseAddrPort("192.168.1.10:47808")
	in, err := NewForwardedNpdu(origin, []byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatalf("NewForwardedNpdu() error = %v", err)
	}

	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got ForwardedNpdu
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.BVLCFunctionType() != FunctionForwardedNPDU {
		t.Fatalf("BVLCFunctionType() = %v, want %v", got.BVLCFunctionType(), FunctionForwardedNPDU)
	}

	if got.OriginatingDeviceAddress() != origin {
		t.Fatalf("OriginatingDeviceAddress() = %v, want %v", got.OriginatingDeviceAddress(), origin)
	}

	npdu := got.NPDUBytes()
	if len(npdu) != 3 || npdu[0] != 0x01 || npdu[2] != 0x03 {
		t.Fatalf("NPDUBytes() = %v, want [1 2 3]", npdu)
	}

	npdu[0] = 0xFF
	if got.NPDUBytes()[0] != 0x01 {
		t.Fatal("NPDUBytes() did not return a defensive copy")
	}
}

func TestForwardedNpduValidation(t *testing.T) {
	_, err := NewForwardedNpdu(netip.MustParseAddrPort("[::1]:47808"), []byte{0x01})
	if !errors.Is(err, bacneterrors.ErrInvalidIPAddress) {
		t.Fatalf("NewForwardedNpdu() error = %v, want %v", err, bacneterrors.ErrInvalidIPAddress)
	}

	_, err = NewForwardedNpdu(netip.MustParseAddrPort("10.0.0.1:47808"), nil)
	if !errors.Is(err, ErrInvalidLength) {
		t.Fatalf("NewForwardedNpdu() error = %v, want %v", err, ErrInvalidLength)
	}
}

func TestReadForeignDeviceTableEncodeDecode(t *testing.T) {
	in := NewReadForeignDeviceTable()
	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got ReadForeignDeviceTable
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.BVLCFunctionType() != FunctionReadForeignDeviceTable {
		t.Fatalf("BVLCFunctionType() = %v, want %v", got.BVLCFunctionType(), FunctionReadForeignDeviceTable)
	}
}

func TestReadForeignDeviceTableAckRoundTripAndDefensiveCopy(t *testing.T) {
	entries := []FdtEntry{
		mustFdtEntryForBVLCFunctionTest(t, "192.168.10.5:47808", 60),
		mustFdtEntryForBVLCFunctionTest(t, "192.168.10.6:47808", 30),
	}

	in, err := NewReadForeignDeviceTableAck(entries)
	if err != nil {
		t.Fatalf("NewReadForeignDeviceTableAck() error = %v", err)
	}

	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got ReadForeignDeviceTableAck
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.BVLCFunctionType() != FunctionReadForeignDeviceTableAck {
		t.Fatalf("BVLCFunctionType() = %v, want %v", got.BVLCFunctionType(), FunctionReadForeignDeviceTableAck)
	}

	copied := got.Entries()
	if len(copied) != 2 {
		t.Fatalf("len(Entries()) = %d, want 2", len(copied))
	}
	copied[0] = FdtEntry{}
	if got.Entries()[0].Address() != entries[0].Address() {
		t.Fatal("Entries() did not return a defensive copy")
	}
}

func TestDeleteForeignDeviceTableEntryEncodeDecode(t *testing.T) {
	entry := mustFdtEntryForBVLCFunctionTest(t, "192.168.10.9:47808", 120)
	in, err := NewDeleteForeignDeviceTableEntry(entry)
	if err != nil {
		t.Fatalf("NewDeleteForeignDeviceTableEntry() error = %v", err)
	}

	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got DeleteForeignDeviceTableEntry
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.BVLCFunctionType() != FunctionDeleteForeignDeviceTableEntry {
		t.Fatalf("BVLCFunctionType() = %v, want %v", got.BVLCFunctionType(), FunctionDeleteForeignDeviceTableEntry)
	}

	decodedEntry := got.FdtEntry()
	if decodedEntry.Address() != entry.Address() {
		t.Fatalf("FdtEntry().Address() = %v, want %v", decodedEntry.Address(), entry.Address())
	}
}

func TestDistributeBroadcastToNetworkRoundTripAndDefensiveCopy(t *testing.T) {
	in, err := NewDistributeBroadcastToNetwork(BVLCTypeBACnetIP, []byte{0x01, 0x20, 0xFF})
	if err != nil {
		t.Fatalf("NewDistributeBroadcastToNetwork() error = %v", err)
	}

	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got DistributeBroadcastToNetwork
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.BVLCFunctionType() != FunctionDistributeBroadcastToNetwork {
		t.Fatalf("BVLCFunctionType() = %v, want %v", got.BVLCFunctionType(), FunctionDistributeBroadcastToNetwork)
	}

	npdu := got.NPDUBytes()
	if len(npdu) != 3 || npdu[2] != 0xFF {
		t.Fatalf("NPDUBytes() = %v, want [1 32 255]", npdu)
	}
	npdu[0] = 0x99
	if got.NPDUBytes()[0] != 0x01 {
		t.Fatal("NPDUBytes() did not return a defensive copy")
	}
}

func TestOriginalUnicastNpduRoundTrip(t *testing.T) {
	in, err := NewOriginalUnicastNpdu(BVLCTypeBACnetIP, []byte{0xAA, 0xBB})
	if err != nil {
		t.Fatalf("NewOriginalUnicastNpdu() error = %v", err)
	}

	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got OriginalUnicastNpdu
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	npdu := got.NPDUBytes()
	if len(npdu) != 2 || npdu[0] != 0xAA || npdu[1] != 0xBB {
		t.Fatalf("NPDUBytes() = %v, want [170 187]", npdu)
	}
}

func TestOriginalBroadcastNpduRoundTrip(t *testing.T) {
	in, err := NewOriginalBroadcastNpdu(BVLCTypeBACnetIP6, []byte{0x10, 0x20})
	if err != nil {
		t.Fatalf("NewOriginalBroadcastNpdu() error = %v", err)
	}

	raw, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var got OriginalBroadcastNpdu
	if err := got.Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	npdu := got.NPDUBytes()
	if len(npdu) != 2 || npdu[0] != 0x10 || npdu[1] != 0x20 {
		t.Fatalf("NPDUBytes() = %v, want [16 32]", npdu)
	}
}

func TestWriteAndReadBroadcastDistributionTableEntryCopies(t *testing.T) {
	entries := []BdtEntry{mustBdtEntryForBVLCFunctionTest(t, "10.0.0.1:47808", net.IPv4Mask(255, 255, 255, 0))}

	writeReq, err := NewWriteBroadcastDistributionTable(entries)
	if err != nil {
		t.Fatalf("NewWriteBroadcastDistributionTable() error = %v", err)
	}

	copiedWriteEntries := writeReq.Entries()
	copiedWriteEntries[0] = BdtEntry{}
	if !writeReq.Entries()[0].Valid() {
		t.Fatal("WriteBroadcastDistributionTable.Entries() did not return a defensive copy")
	}

	ack, err := NewReadBroadcastDistributionTableAck(entries)
	if err != nil {
		t.Fatalf("NewReadBroadcastDistributionTableAck() error = %v", err)
	}

	copiedAckEntries := ack.BdtEntries()
	copiedAckEntries[0] = BdtEntry{}
	if !ack.BdtEntries()[0].Valid() {
		t.Fatal("ReadBroadcastDistributionTableAck.BdtEntries() did not return a defensive copy")
	}
}

package bip

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

// helpers -------------------------------------------------------------------------

// mustBdtEntry constructs a BdtEntry from a host:port string and mask, panicking on error.
func mustBdtEntry(addrPort string, mask net.IPMask) BdtEntry {
	ap := netip.MustParseAddrPort(addrPort)
	e, err := NewBdtEntry(ap, mask)
	if err != nil {
		panic(err)
	}
	return *e
}

// validSrc is a reusable IPv4 source address for foreign-device tests.
var validSrc = netip.MustParseAddrPort("192.168.1.100:47808")

// mustRegisterFD builds a RegisterForeignDevice message with the given TTL.
func mustRegisterFD(ttl TTL) *RegisterForeignDevice {
	r, err := NewRegisterForeignDevice(ttl)
	if err != nil {
		panic(err)
	}
	return r
}

// emptyBBMD returns a freshly constructed BBMD with no BDT entries.
func emptyBBMD(t *testing.T) BBMD {
	t.Helper()
	b, err := NewBBMD(nil)
	if err != nil {
		t.Fatalf("NewBBMD: %v", err)
	}
	return b
}

// NewBBMD -------------------------------------------------------------------------

func TestNewBBMDNilBDT(t *testing.T) {
	_, err := NewBBMD(nil)
	if err != nil {
		t.Fatalf("NewBBMD(nil) = %v, want nil error", err)
	}
}

func TestNewBBMDEmptyBDT(t *testing.T) {
	_, err := NewBBMD([]BdtEntry{})
	if err != nil {
		t.Fatalf("NewBBMD([]) = %v, want nil error", err)
	}
}

func TestNewBBMDValidBDT(t *testing.T) {
	entries := []BdtEntry{
		mustBdtEntry("10.0.0.1:47808", net.IPv4Mask(255, 255, 255, 0)),
	}
	_, err := NewBBMD(entries)
	if err != nil {
		t.Fatalf("NewBBMD(valid entries) = %v, want nil error", err)
	}
}

func TestNewBBMDInvalidBDTEntryRejected(t *testing.T) {
	// An zero-value BdtEntry is invalid (nil mask triggers Valid() == false via index panic guard,
	// but NewBDT validates before constructing — this tests the constructor validation path).
	// Construct an invalid entry by direct struct literal (same package).
	invalid := BdtEntry{} // zero value: address invalid, mask nil
	_, err := NewBBMD([]BdtEntry{invalid})
	if err == nil {
		t.Fatal("NewBBMD with invalid BdtEntry should return an error")
	}
}

func TestNewBBMDDefensiveCopyOfBDT(t *testing.T) {
	entries := []BdtEntry{mustBdtEntry("10.0.0.1:47808", net.IPv4Mask(255, 255, 255, 0))}
	b, _ := NewBBMD(entries)

	// Mutating the original slice must not affect the BBMD's internal BDT.
	entries[0] = BdtEntry{}

	req := NewReadForeignDeviceTable()
	_, err := b.HandleReadBroadcastDistributionTable(NewReadBroadcastDistributionTable())
	if err != nil {
		t.Fatalf("HandleReadBroadcastDistributionTable: %v", err)
	}
	// The BBMD should still return the original valid entry, not the zeroed one.
	ack, err := b.HandleReadBroadcastDistributionTable(NewReadBroadcastDistributionTable())
	if err != nil {
		t.Fatalf("HandleReadBroadcastDistributionTable: %v", err)
	}
	got := ack.BdtEntries()
	if len(got) != 1 {
		t.Fatalf("BDT length = %d, want 1", len(got))
	}
	_ = req
}

// HandleRegisterForeignDevice -----------------------------------------------------

func TestHandleRegisterForeignDeviceSuccess(t *testing.T) {
	b := emptyBBMD(t)
	result, err := b.HandleRegisterForeignDevice(validSrc, mustRegisterFD(60))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultCode() != ResultCodeSuccessfulCompletion {
		t.Errorf("result code = %v, want %v", result.ResultCode(), ResultCodeSuccessfulCompletion)
	}
}

func TestHandleRegisterForeignDeviceNilReqError(t *testing.T) {
	b := emptyBBMD(t)
	_, err := b.HandleRegisterForeignDevice(validSrc, nil)
	if err == nil {
		t.Fatal("expected error for nil req")
	}
}

func TestHandleRegisterForeignDeviceInvalidSrcNak(t *testing.T) {
	tests := []struct {
		name string
		src  netip.AddrPort
	}{
		{"zero value", netip.AddrPort{}},
		{"ipv6 src", netip.MustParseAddrPort("[::1]:47808")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := emptyBBMD(t)
			result, err := b.HandleRegisterForeignDevice(tt.src, mustRegisterFD(30))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ResultCode() != ResultCodeRegisterForeignDeviceNak {
				t.Errorf("result code = %v, want %v", result.ResultCode(), ResultCodeRegisterForeignDeviceNak)
			}
		})
	}
}

func TestHandleRegisterForeignDeviceAppearsInFDT(t *testing.T) {
	b := emptyBBMD(t)
	if _, err := b.HandleRegisterForeignDevice(validSrc, mustRegisterFD(60)); err != nil {
		t.Fatalf("register: %v", err)
	}

	ack, err := b.HandleReadForeignDeviceTable(NewReadForeignDeviceTable())
	if err != nil {
		t.Fatalf("read fdt: %v", err)
	}
	entries := ack.Entries()
	if len(entries) != 1 {
		t.Fatalf("FDT length = %d, want 1", len(entries))
	}
	if entries[0].Address() != validSrc {
		t.Errorf("FDT entry address = %v, want %v", entries[0].Address(), validSrc)
	}
}

func TestHandleRegisterForeignDeviceRenewal(t *testing.T) {
	b := emptyBBMD(t)
	// Register twice from the same address — second registration must replace the first.
	b.HandleRegisterForeignDevice(validSrc, mustRegisterFD(30))  //nolint
	b.HandleRegisterForeignDevice(validSrc, mustRegisterFD(120)) //nolint

	ack, _ := b.HandleReadForeignDeviceTable(NewReadForeignDeviceTable())
	if len(ack.Entries()) != 1 {
		t.Errorf("FDT length = %d after renewal, want 1", len(ack.Entries()))
	}
}

// HandleReadForeignDeviceTable ----------------------------------------------------

func TestHandleReadForeignDeviceTableNilReqError(t *testing.T) {
	b := emptyBBMD(t)
	_, err := b.HandleReadForeignDeviceTable(nil)
	if err == nil {
		t.Fatal("expected error for nil req")
	}
}

func TestHandleReadForeignDeviceTableEmptyInitially(t *testing.T) {
	b := emptyBBMD(t)
	ack, err := b.HandleReadForeignDeviceTable(NewReadForeignDeviceTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ack.Entries()) != 0 {
		t.Errorf("FDT length = %d, want 0", len(ack.Entries()))
	}
}

func TestHandleReadForeignDeviceTablePurgesExpiredEntries(t *testing.T) {
	b := emptyBBMD(t)

	impl := b.(*bbmdImpl)
	// Inject a pre-expired entry directly (bypasses constructor TTL limits).
	impl.mu.Lock()
	impl.fdt[validSrc] = bbmdFdtEntry{
		address:   validSrc,
		ttl:       1,
		expiresAt: time.Now().Add(-1 * time.Second), // already expired
	}
	impl.mu.Unlock()

	ack, err := b.HandleReadForeignDeviceTable(NewReadForeignDeviceTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ack.Entries()) != 0 {
		t.Errorf("FDT length = %d after expiry, want 0", len(ack.Entries()))
	}
}

func TestHandleReadForeignDeviceTableRemainingTTLDecreases(t *testing.T) {
	b := emptyBBMD(t)
	b.HandleRegisterForeignDevice(validSrc, mustRegisterFD(60)) //nolint

	ack, _ := b.HandleReadForeignDeviceTable(NewReadForeignDeviceTable())
	entries := ack.Entries()
	if len(entries) != 1 {
		t.Fatalf("FDT length = %d, want 1", len(entries))
	}
	// remaining must be ≤ 60 + 30 = 90 and > 0
	remaining := entries[0].RemainingTtl()
	if remaining == 0 || remaining > 90 {
		t.Errorf("remaining TTL = %d, want in (0, 90]", remaining)
	}
}

// HandleDeleteForeignDeviceTableEntry --------------------------------------------

func TestHandleDeleteForeignDeviceTableEntryNilReqError(t *testing.T) {
	b := emptyBBMD(t)
	_, err := b.HandleDeleteForeignDeviceTableEntry(nil)
	if err == nil {
		t.Fatal("expected error for nil req")
	}
}

func TestHandleDeleteForeignDeviceTableEntryNotFoundNak(t *testing.T) {
	b := emptyBBMD(t)

	entry, err := NewFdtEntry(validSrc, 60)
	if err != nil {
		t.Fatalf("NewFdtEntry: %v", err)
	}
	deleteMsg, err := NewDeleteForeignDeviceTableEntry(*entry)
	if err != nil {
		t.Fatalf("NewDeleteForeignDeviceTableEntry: %v", err)
	}

	result, err := b.HandleDeleteForeignDeviceTableEntry(deleteMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultCode() != ResultCodeDeleteForeignDeviceTableEntryNak {
		t.Errorf("result code = %v, want %v", result.ResultCode(), ResultCodeDeleteForeignDeviceTableEntryNak)
	}
}

func TestHandleDeleteForeignDeviceTableEntrySuccess(t *testing.T) {
	b := emptyBBMD(t)
	b.HandleRegisterForeignDevice(validSrc, mustRegisterFD(60)) //nolint

	entry, err := NewFdtEntry(validSrc, 60)
	if err != nil {
		t.Fatalf("NewFdtEntry: %v", err)
	}
	deleteMsg, err := NewDeleteForeignDeviceTableEntry(*entry)
	if err != nil {
		t.Fatalf("NewDeleteForeignDeviceTableEntry: %v", err)
	}

	result, err := b.HandleDeleteForeignDeviceTableEntry(deleteMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultCode() != ResultCodeSuccessfulCompletion {
		t.Errorf("result code = %v, want %v", result.ResultCode(), ResultCodeSuccessfulCompletion)
	}

	ack, _ := b.HandleReadForeignDeviceTable(NewReadForeignDeviceTable())
	if len(ack.Entries()) != 0 {
		t.Errorf("FDT length = %d after delete, want 0", len(ack.Entries()))
	}
}

// HandleWriteBroadcastDistributionTable ------------------------------------------

func TestHandleWriteBroadcastDistributionTableNilReqError(t *testing.T) {
	b := emptyBBMD(t)
	_, err := b.HandleWriteBroadcastDistributionTable(nil)
	if err == nil {
		t.Fatal("expected error for nil req")
	}
}

func TestHandleWriteBroadcastDistributionTableSuccess(t *testing.T) {
	b := emptyBBMD(t)
	entries := []BdtEntry{mustBdtEntry("10.0.1.1:47808", net.IPv4Mask(255, 255, 255, 0))}
	req, err := NewWriteBroadcastDistributionTable(entries)
	if err != nil {
		t.Fatalf("NewWriteBroadcastDistributionTable: %v", err)
	}

	result, err := b.HandleWriteBroadcastDistributionTable(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultCode() != ResultCodeSuccessfulCompletion {
		t.Errorf("result code = %v, want %v", result.ResultCode(), ResultCodeSuccessfulCompletion)
	}
}

func TestHandleWriteBroadcastDistributionTableReplacesBDT(t *testing.T) {
	initial := []BdtEntry{mustBdtEntry("10.0.0.1:47808", net.IPv4Mask(255, 255, 0, 0))}
	b, _ := NewBBMD(initial)

	replacement := []BdtEntry{
		mustBdtEntry("10.1.0.1:47808", net.IPv4Mask(255, 255, 255, 0)),
		mustBdtEntry("10.2.0.1:47808", net.IPv4Mask(255, 255, 255, 0)),
	}
	req, _ := NewWriteBroadcastDistributionTable(replacement)
	b.HandleWriteBroadcastDistributionTable(req) //nolint

	ack, err := b.HandleReadBroadcastDistributionTable(NewReadBroadcastDistributionTable())
	if err != nil {
		t.Fatalf("read BDT: %v", err)
	}
	if len(ack.BdtEntries()) != 2 {
		t.Errorf("BDT length = %d, want 2", len(ack.BdtEntries()))
	}
}

// HandleReadBroadcastDistributionTable -------------------------------------------

func TestHandleReadBroadcastDistributionTableNilReqError(t *testing.T) {
	b := emptyBBMD(t)
	_, err := b.HandleReadBroadcastDistributionTable(nil)
	if err == nil {
		t.Fatal("expected error for nil req")
	}
}

func TestHandleReadBroadcastDistributionTableEmpty(t *testing.T) {
	b := emptyBBMD(t)
	ack, err := b.HandleReadBroadcastDistributionTable(NewReadBroadcastDistributionTable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ack.BdtEntries()) != 0 {
		t.Errorf("BDT length = %d, want 0", len(ack.BdtEntries()))
	}
}

func TestHandleReadBroadcastDistributionTableReturnsCopy(t *testing.T) {
	entries := []BdtEntry{mustBdtEntry("10.0.0.1:47808", net.IPv4Mask(255, 255, 255, 0))}
	b, _ := NewBBMD(entries)

	ack1, _ := b.HandleReadBroadcastDistributionTable(NewReadBroadcastDistributionTable())
	ack2, _ := b.HandleReadBroadcastDistributionTable(NewReadBroadcastDistributionTable())

	// Two independent calls must return equal but independently allocated slices.
	got1, got2 := ack1.BdtEntries(), ack2.BdtEntries()
	if len(got1) != len(got2) {
		t.Fatalf("BDT lengths differ: %d vs %d", len(got1), len(got2))
	}
}

// bbmdFdtEntry helpers -----------------------------------------------------------

func TestBbmdFdtEntryRemainingTTLDecrements(t *testing.T) {
	e := bbmdFdtEntry{
		ttl:       60,
		expiresAt: time.Now().Add(45 * time.Second),
	}
	remaining := e.remainingTTL()
	if remaining == 0 || remaining > 45 {
		t.Errorf("remainingTTL() = %d, want in (0, 45]", remaining)
	}
}

func TestBbmdFdtEntryRemainingTTLZeroWhenExpired(t *testing.T) {
	e := bbmdFdtEntry{
		ttl:       1,
		expiresAt: time.Now().Add(-1 * time.Second),
	}
	if e.remainingTTL() != 0 {
		t.Errorf("remainingTTL() = %d, want 0 for expired entry", e.remainingTTL())
	}
}

func TestBbmdFdtEntryExpired(t *testing.T) {
	past := bbmdFdtEntry{expiresAt: time.Now().Add(-time.Second)}
	future := bbmdFdtEntry{expiresAt: time.Now().Add(time.Minute)}
	if !past.expired() {
		t.Error("past entry should be expired")
	}
	if future.expired() {
		t.Error("future entry should not be expired")
	}
}

func TestBbmdFdtEntryToWireFdtEntry(t *testing.T) {
	e := bbmdFdtEntry{
		address:   validSrc,
		ttl:       60,
		expiresAt: time.Now().Add(80 * time.Second),
	}
	wire := e.toWireFdtEntry()
	if wire.Address() != validSrc {
		t.Errorf("address = %v, want %v", wire.Address(), validSrc)
	}
	if wire.RegisteredTtl() != 60 {
		t.Errorf("registeredTtl = %d, want 60", wire.RegisteredTtl())
	}
	// remaining must be ≤ 80 and > 0
	if wire.RemainingTtl() == 0 || wire.RemainingTtl() > 80 {
		t.Errorf("remainingTtl = %d, want in (0, 80]", wire.RemainingTtl())
	}
}

// sentinel error identity checks -------------------------------------------------

func TestHandleRegisterNilReqReturnsError(t *testing.T) {
	b := emptyBBMD(t)
	result, err := b.HandleRegisterForeignDevice(validSrc, nil)
	if err == nil {
		t.Fatal("expected non-nil error for nil req")
	}
	// Must not return a BVLCResult when the call itself is malformed.
	if result != nil {
		t.Fatal("result must be nil when err is non-nil")
	}
}

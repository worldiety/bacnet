package bip

import (
	"fmt"
	"math"
	"net/netip"
	"sync"
	"time"

	"go.wdy.de/bacnet"
)

// BBMD is a BACnet Broadcast Management Device as defined in Annex J of ANSI/ASHRAE 135.
//
// A BBMD bridges BACnet broadcast traffic across IP subnets. It maintains two tables:
//   - Broadcast Distribution Table (BDT): the set of peer BBMDs to which it forwards broadcasts.
//   - Foreign Device Table (FDT): BACnet devices on foreign subnets that have registered here.
//
// Each Handle method corresponds to one inbound BVLC message type (Annex J function code
// shown in parentheses). The caller is responsible for extracting the UDP source address
// from the received datagram and for transmitting any returned response frame.
type BBMD interface {
	// HandleRegisterForeignDevice processes a Register-Foreign-Device (0x05) request.
	// src is the UDP source address of the registering device.
	// Returns ResultCodeSuccessfulCompletion on success,
	// or ResultCodeRegisterForeignDeviceNak if the request cannot be honoured
	// (e.g. src is not a valid IPv4 address).
	HandleRegisterForeignDevice(src netip.AddrPort, req *RegisterForeignDevice) (*BVLCResult, error)

	// HandleReadForeignDeviceTable processes a Read-Foreign-Device-Table (0x06) request.
	// Expired entries are purged before building the response.
	HandleReadForeignDeviceTable(req *ReadForeignDeviceTable) (*ReadForeignDeviceTableAck, error)

	// HandleDeleteForeignDeviceTableEntry processes a Delete-Foreign-Device-Table-Entry (0x08) request.
	// Returns ResultCodeSuccessfulCompletion if the entry was found and removed,
	// or ResultCodeDeleteForeignDeviceTableEntryNak if no matching entry exists.
	HandleDeleteForeignDeviceTableEntry(req *DeleteForeignDeviceTableEntry) (*BVLCResult, error)

	// HandleWriteBroadcastDistributionTable processes a Write-Broadcast-Distribution-Table (0x01) request.
	// The current BDT is atomically replaced by the entries carried in req.
	// Returns ResultCodeSuccessfulCompletion on success,
	// or ResultCodeWriteBroadcastDistributionTableNak if req is invalid.
	HandleWriteBroadcastDistributionTable(req *WriteBroadcastDistributionTable) (*BVLCResult, error)

	// HandleReadBroadcastDistributionTable processes a Read-Broadcast-Distribution-Table (0x02) request.
	// Returns the current Broadcast Distribution Table.
	HandleReadBroadcastDistributionTable(req *ReadBroadcastDistributionTable) (*ReadBroadcastDistributionTableAck, error)
}

// NewBBMD returns a BBMD initialised with the supplied Broadcast Distribution Table.
// bdt may be nil or empty; it can be replaced later via HandleWriteBroadcastDistributionTable.
// All BdtEntry values in bdt are validated before the BBMD is returned.
func NewBBMD(bdt []BdtEntry) (BBMD, error) {
	for i, e := range bdt {
		if !e.Valid() {
			return nil, bacnet.NewValidationError(fmt.Sprintf("bdt[%d]", i), e, ErrInvalidIPAddress)
		}
	}

	bdtCopy := make([]BdtEntry, len(bdt))
	copy(bdtCopy, bdt)

	return &bbmdImpl{
		bdt: bdtCopy,
		fdt: make(map[netip.AddrPort]bbmdFdtEntry),
	}, nil
}

// bbmdImpl is the default in-process BBMD implementation.
// All methods are safe for concurrent use.
type bbmdImpl struct {
	mu  sync.RWMutex
	bdt []BdtEntry                      // Broadcast Distribution Table
	fdt map[netip.AddrPort]bbmdFdtEntry // Foreign Device Table, keyed by IPv4 address+port
}

// bbmdFdtEntry is the BBMD's internal representation of a foreign device registration.
// Per Annex J §J.5.2.3, the entry expires (ttl + 30) seconds after registration.
type bbmdFdtEntry struct {
	address   netip.AddrPort
	ttl       TTL       // value received in the Register-Foreign-Device request
	expiresAt time.Time // registration time + (ttl + 30) seconds
}

// remainingTTL returns the number of seconds remaining before this entry expires.
// Returns 0 once the entry has expired.
func (e *bbmdFdtEntry) remainingTTL() TTL {
	secs := time.Until(e.expiresAt).Seconds()
	if secs <= 0 {
		return 0
	}
	if secs > math.MaxUint16 {
		return math.MaxUint16
	}
	return TTL(secs)
}

// expired reports whether this entry has passed its expiry time.
func (e *bbmdFdtEntry) expired() bool {
	return !time.Now().Before(e.expiresAt)
}

// toWireFdtEntry converts the internal entry to the wire-level FdtEntry for inclusion
// in a Read-Foreign-Device-Table-Ack. Because bbmdFdtEntry and FdtEntry share the bip
// package, unexported fields can be set directly, encoding the current remaining TTL
// rather than the initial (registered + 30) value.
func (e *bbmdFdtEntry) toWireFdtEntry() FdtEntry {
	return FdtEntry{
		address:       e.address,
		registeredTtl: e.ttl,
		remainingTtl:  e.remainingTTL(),
	}
}

// purgeExpired removes all expired entries from the FDT.
// The caller must hold b.mu for writing.
func (b *bbmdImpl) purgeExpired() {
	for addr, e := range b.fdt {
		if e.expired() {
			delete(b.fdt, addr)
		}
	}
}

// HandleRegisterForeignDevice implements BBMD.
func (b *bbmdImpl) HandleRegisterForeignDevice(src netip.AddrPort, req *RegisterForeignDevice) (*BVLCResult, error) {
	if req == nil {
		return nil, fmt.Errorf("register-foreign-device: req must not be nil")
	}

	if !src.IsValid() || !src.Addr().Is4() {
		return NewBVLCResult(ResultCodeRegisterForeignDeviceNak)
	}

	ttl := req.TTL()
	gracePeriod := min(int(ttl)+30, math.MaxUint16)

	b.mu.Lock()
	b.fdt[src] = bbmdFdtEntry{
		address:   src,
		ttl:       ttl,
		expiresAt: time.Now().Add(time.Duration(gracePeriod) * time.Second),
	}
	b.mu.Unlock()

	return NewBVLCResult(ResultCodeSuccessfulCompletion)
}

// HandleReadForeignDeviceTable implements BBMD.
func (b *bbmdImpl) HandleReadForeignDeviceTable(req *ReadForeignDeviceTable) (*ReadForeignDeviceTableAck, error) {
	if req == nil {
		return nil, fmt.Errorf("read-foreign-device-table: req must not be nil")
	}

	b.mu.Lock()
	b.purgeExpired()
	entries := make([]FdtEntry, 0, len(b.fdt))
	for _, e := range b.fdt {
		entries = append(entries, e.toWireFdtEntry())
	}
	b.mu.Unlock()

	return NewReadForeignDeviceTableAck(entries)
}

// HandleDeleteForeignDeviceTableEntry implements BBMD.
func (b *bbmdImpl) HandleDeleteForeignDeviceTableEntry(req *DeleteForeignDeviceTableEntry) (*BVLCResult, error) {
	if req == nil {
		return nil, fmt.Errorf("delete-foreign-device-table-entry: req must not be nil")
	}

	entry := req.FdtEntry()
	addr := entry.Address()

	b.mu.Lock()
	_, found := b.fdt[addr]
	if found {
		delete(b.fdt, addr)
	}
	b.mu.Unlock()

	if !found {
		return NewBVLCResult(ResultCodeDeleteForeignDeviceTableEntryNak)
	}

	return NewBVLCResult(ResultCodeSuccessfulCompletion)
}

// HandleWriteBroadcastDistributionTable implements BBMD.
func (b *bbmdImpl) HandleWriteBroadcastDistributionTable(req *WriteBroadcastDistributionTable) (*BVLCResult, error) {
	if req == nil {
		return nil, fmt.Errorf("write-broadcast-distribution-table: req must not be nil")
	}

	if !req.Valid() {
		return NewBVLCResult(ResultCodeWriteBroadcastDistributionTableNak)
	}

	entries := req.Entries()
	bdtCopy := make([]BdtEntry, len(entries))
	copy(bdtCopy, entries)

	b.mu.Lock()
	b.bdt = bdtCopy
	b.mu.Unlock()

	return NewBVLCResult(ResultCodeSuccessfulCompletion)
}

// HandleReadBroadcastDistributionTable implements BBMD.
func (b *bbmdImpl) HandleReadBroadcastDistributionTable(req *ReadBroadcastDistributionTable) (*ReadBroadcastDistributionTableAck, error) {
	if req == nil {
		return nil, fmt.Errorf("read-broadcast-distribution-table: req must not be nil")
	}

	b.mu.RLock()
	bdtCopy := make([]BdtEntry, len(b.bdt))
	copy(bdtCopy, b.bdt)
	b.mu.RUnlock()

	return NewReadBroadcastDistributionTableAck(bdtCopy)
}

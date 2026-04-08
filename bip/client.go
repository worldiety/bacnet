package bip

import (
	"maps"
	"math"
	"net/netip"
	"time"
)

// ClientIp4 defines the interface for a BACnet IP4 client
type ClientIp4 interface { //todo maybe rename to 'device'
	// SendLocalBroadcast sends a broadcast to the local IP subnet. Reaches all nodes on the clients subnet.
	SendLocalBroadcast(msg OriginalBroadcastNpdu) error
	RegisterAsForeignDevice(bbmdAddr netip.Addr) error
}

func NewClientIp4() ClientIp4 {
	return &clientImpl{}
}

type clientImpl struct{}

func (c *clientImpl) SendLocalBroadcast(msg OriginalBroadcastNpdu) error {
	//TODO implement me
	panic("implement me")
}

func (c *clientImpl) RegisterAsForeignDevice(bbmdAddr netip.Addr) error {
	panic("implement me")
}

type BacnetBroadcastManagementDevice interface {
	RegisterForeignDevice(addr netip.AddrPort, msg RegisterForeignDevice) error
	ReadBroadcastDistributionTable(msg ReadBroadcastDistributionTable) (ReadBroadcastDistributionTableAck, error)
	WriteBroadcastDistributionTable(msg WriteBroadcastDistributionTable) error
}

func NewBacnetBroadcastManagementDevice() BacnetBroadcastManagementDevice {
	return &bbmdImpl{
		foreignDeviceTable: make(foreignDeviceTable),
	}
}

type bbmdImpl struct {
	foreignDeviceTable foreignDeviceTable
}

func (b *bbmdImpl) RegisterForeignDevice(addr netip.AddrPort, msg RegisterForeignDevice) error {
	newEntry := ForeignDeviceTableEntry{
		addr:      addr,
		ttl:       msg.ttl,
		createdAt: time.Now(),
	}

	b.foreignDeviceTable.Add(newEntry)

	return nil
}

func (b *bbmdImpl) ReadBroadcastDistributionTable(msg ReadBroadcastDistributionTable) (ReadBroadcastDistributionTableAck, error) {
	//TODO implement me
	panic("implement me")
}

func (b *bbmdImpl) WriteBroadcastDistributionTable(msg WriteBroadcastDistributionTable) error {
	//TODO implement me
	panic("implement me")
}

type ForeignDeviceTableEntry struct {
	addr      netip.AddrPort
	ttl       TTL
	createdAt time.Time
}

func NewForeignDeviceTableEntry(addr netip.AddrPort, ttl TTL) *ForeignDeviceTableEntry {
	deletionIn := uint16(30) + uint16(ttl)
	if deletionIn > math.MaxUint16 { // max 65535
		deletionIn = math.MaxUint16
	}

	return &ForeignDeviceTableEntry{
		addr:      addr,
		ttl:       ttl,
		createdAt: time.Now(),
	}
}

func (e *ForeignDeviceTableEntry) Addr() netip.AddrPort {
	return e.addr
}

func (e *ForeignDeviceTableEntry) TTL() TTL {
	return e.ttl
}

func (e *ForeignDeviceTableEntry) DeletionIn() uint16 {
	deletionIn := e.createdAt.Add(30 * time.Second).Add(-time.Duration(e.ttl) * time.Second).Second()

	if deletionIn > math.MaxUint16 { //check for overflow
		deletionIn = math.MaxUint16
	} else if deletionIn < 0 {
		deletionIn = 0
	}

	return uint16(deletionIn)
}

type foreignDeviceTable map[netip.AddrPort]ForeignDeviceTableEntry

func (f *foreignDeviceTable) UpdateForeignDeviceTable(newEntries []ForeignDeviceTableEntry) {
	for _, entry := range newEntries {
		(*f)[entry.addr] = entry
	}

	maps.DeleteFunc(*f, func(k netip.AddrPort, v ForeignDeviceTableEntry) bool {
		return v.DeletionIn() == 0
	})
}

func (f *foreignDeviceTable) DeleteForeignDevice(msg FdtEntry) bool {
	found := false
	maps.DeleteFunc(*f, func(k netip.AddrPort, v ForeignDeviceTableEntry) bool {
		del := k == msg.Address()
		found = found || del
		return del
	})

	return found
}

func (f *foreignDeviceTable) Add(entry ForeignDeviceTableEntry) {
	(*f)[entry.addr] = entry
}

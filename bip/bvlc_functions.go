package bip

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"time"

	"go.wdy.de/bacnet"
)

type BVLCResultCode uint16

const (
	ResultCodeSuccessfulCompletion               BVLCResultCode = 0x000
	ResultCodeWriteBroadcastDistributionTableNak BVLCResultCode = 0x0010
	ResultCodeReadBroadcastDistributionTableNak  BVLCResultCode = 0x0020
	ResultCodeRegisterForeignDeviceNak           BVLCResultCode = 0x0030
	ResultCodeReadForeignDeviceTableNak          BVLCResultCode = 0x0040
	ResultCodeDeleteForeignDeviceTableEntryNak   BVLCResultCode = 0x0050
	ResultCodeDistributeBroadcastToNetworkNak    BVLCResultCode = 0x0060
)

func (b BVLCResultCode) Valid() bool {
	switch b {
	case ResultCodeSuccessfulCompletion:
		fallthrough
	case ResultCodeWriteBroadcastDistributionTableNak:
		fallthrough
	case ResultCodeReadBroadcastDistributionTableNak:
		fallthrough
	case ResultCodeRegisterForeignDeviceNak:
		fallthrough
	case ResultCodeReadForeignDeviceTableNak:
		fallthrough
	case ResultCodeDeleteForeignDeviceTableEntryNak:
		fallthrough
	case ResultCodeDistributeBroadcastToNetworkNak:
		return true
	default:
		return false
	}
}

type BVLCResult struct {
	header     BVLCHeader
	resultCode BVLCResultCode
}

func (r *BVLCResult) BVLCFunctionType() BVLCFunctionType {
	return r.header.BVLCFunctionType
}

func (r *BVLCResult) Valid() bool {
	if r == nil {
		return false
	}

	return r.header.Valid() && r.header.BVLCFunctionType == FunctionResult && r.resultCode.Valid()
}

func (r *BVLCResult) Encode() ([]byte, error) {
	const resultFrameLen = BVLCHeaderLen + 2

	if r == nil {
		return nil, fmt.Errorf("nil bvlc-result")
	}

	if r.header.BVLCLength != resultFrameLen {
		return nil, fmt.Errorf("invalid bvlc-length for bvlc-result: %d", r.header.BVLCLength)
	}

	out, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-result: %w", err)
	}

	code := uint16(r.resultCode)
	out[4] = byte(code >> 8)
	out[5] = byte(code)

	return out, nil
}

// Decode decodes a BVLC Result frame from wire bytes.
func (r *BVLCResult) Decode(data []byte) error {
	const resultFrameLen = BVLCHeaderLen + 2

	if len(data) != resultFrameLen {
		return bacnet.NewValidationError("data", len(data), ErrInvalidLength)
	}

	var header BVLCHeader

	headerBytes := data[:BVLCHeaderLen]

	err := header.Decode(headerBytes)
	if err != nil {
		return fmt.Errorf("decode bvlc-result header: %w", err)
	}

	if !header.IsIp4() {
		return fmt.Errorf("invalid bvlc-type: %d", header.BVLCType)
	}

	if header.BVLCFunctionType != FunctionResult {
		return fmt.Errorf("invalid bvlc-function for result: %d", header.BVLCFunctionType)
	}

	resultCode := BVLCResultCode(uint16(data[4])<<8 | uint16(data[5]))
	if !resultCode.Valid() {
		return fmt.Errorf("invalid bvlc-result-code: %d", resultCode)
	}

	*r = BVLCResult{
		header:     header,
		resultCode: resultCode,
	}

	return nil
}

// ResultCode returns the BVLC result code carried by the frame.
func (r *BVLCResult) ResultCode() BVLCResultCode {
	return r.resultCode
}

// NewBVLCResult constructs a validated BVLCResult for BACnet/IP (IPv4).
// resultCode must be one of the defined BVLCResultCode constants.
func NewBVLCResult(resultCode BVLCResultCode) (*BVLCResult, error) {
	if !resultCode.Valid() {
		return nil, bacnet.NewValidationError("result code", resultCode, ErrInvalidResultCode)
	}
	const resultFrameLen = BVLCHeaderLen + 2
	return &BVLCResult{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionResult,
			BVLCLength:       BVLCLength(resultFrameLen),
		},
		resultCode: resultCode,
	}, nil
}

type FdtEntry tableEntry

func (f *FdtEntry) Valid() bool {
	return tableEntryValid[FdtEntry](f)
}

func (f *FdtEntry) Encode() ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("nil bvlc-entry")
	}
	return encodeTableEntry[FdtEntry](f)
}

func (f *FdtEntry) Decode(data []byte) error {
	return decodeTableEntry[FdtEntry](data, f)
}

// Address returns the IP address and port of the foreign device.
func (f *FdtEntry) Address() netip.AddrPort {
	return f.address
}

// BroadcastDistributionMask returns a defensive copy of the mask field.
// Note: for FDT entries this field encodes the TTL/remaining-TTL bytes as
// returned by the BBMD; use with awareness of the wire layout.
func (f *FdtEntry) BroadcastDistributionMask() net.IPMask {
	return net.IPMask(cloneBytes(f.broadcastDistributionMask))
}

// NewFdtEntry constructs a validated FdtEntry.
// address must be a valid IPv4 address-port pair.
// broadcastDistributionMask must be exactly 4 bytes.
func NewFdtEntry(address netip.AddrPort, broadcastDistributionMask net.IPMask) (*FdtEntry, error) {
	if !address.Addr().Is4() || !address.IsValid() {
		return nil, bacnet.NewValidationError("address", address, ErrInvalidIPAddress)
	}

	if len(broadcastDistributionMask) != net.IPv4len {
		return nil, bacnet.NewValidationError("broadcast distribution mask", broadcastDistributionMask, ErrInvalidMask)
	}

	entry := &FdtEntry{
		address:                   address,
		broadcastDistributionMask: net.IPMask(cloneBytes(broadcastDistributionMask)),
	}
	if !entry.Valid() {
		return nil, bacnet.NewValidationError("broadcast distribution mask", broadcastDistributionMask, ErrInvalidMask)
	}
	return entry, nil
}

type BdtEntry tableEntry

func (b *BdtEntry) Valid() bool {
	return tableEntryValid[BdtEntry](b)
}

func (b *BdtEntry) Encode() ([]byte, error) {
	return encodeTableEntry[BdtEntry](b)
}

func (b *BdtEntry) Decode(data []byte) error {
	return decodeTableEntry[BdtEntry](data, b)
}

// Address returns the IP address and port of the BDT peer.
func (b *BdtEntry) Address() netip.AddrPort {
	return b.address
}

// BroadcastDistributionMask returns a defensive copy of the subnet broadcast mask.
func (b *BdtEntry) BroadcastDistributionMask() net.IPMask {
	return net.IPMask(cloneBytes(b.broadcastDistributionMask))
}

type tableEntry struct {
	// the IP address of the gateway if NAT is active, of the target otherwise
	address netip.AddrPort
	// the subnet mask if NAT is active, 255.255.255.255 otherwise
	broadcastDistributionMask net.IPMask
}

type tableEntryKinds interface {
	BdtEntry | FdtEntry
}

func newTableEntry[T tableEntryKinds](address netip.AddrPort, broadcastDistributionMask net.IPMask) (*T, error) {
	if !address.Addr().Is4() || !address.IsValid() {
		return nil, bacnet.NewValidationError("address", address, ErrInvalidIPAddress)
	}

	if len(broadcastDistributionMask) != net.IPv4len {
		return nil, bacnet.NewValidationError("broadcast distribution mask", broadcastDistributionMask, ErrInvalidMask)
	}

	return new(T(tableEntry{
		address:                   address,
		broadcastDistributionMask: net.IPMask(cloneBytes(broadcastDistributionMask)),
	})), nil
}

func decodeTableEntry[T tableEntryKinds](data []byte, target *T) error {
	if target == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) != BdtEntryDataLen {
		return fmt.Errorf("invalid length for bdt entry: %d", len(data))
	}

	address, err := decodeAddressPortIpV4(data[0:6])
	if err != nil {
		return fmt.Errorf("invalid ip in bdt entry: %w", err)
	}

	mask := net.IPv4Mask(data[6], data[7], data[8], data[9])

	entry := T(tableEntry{
		address:                   address,
		broadcastDistributionMask: mask,
	})

	if !tableEntryValid[T](&entry) {
		return fmt.Errorf("invalid bdt entry mask") // ip and port are valid, invalidity must be caused by mask
	}

	*target = entry

	return nil
}

func encodeTableEntry[T tableEntryKinds](entry *T) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("cannot encode nil pointer")
	}

	out := make([]byte, BdtEntryDataLen)

	tEntry := tableEntry(*entry)

	if !tEntry.address.Addr().Is4() { //bd entries require ipv4, should be guaranteed by constructor, check here anyway
		return nil, fmt.Errorf("invalid bvlc-address, expected IPv4")
	}

	copy(out[0:6], encodeAddressPortIpV4(tEntry.address))

	copy(out[7:9], tEntry.broadcastDistributionMask)

	return out, nil
}

func tableEntryValid[T tableEntryKinds](entry *T) bool {
	if entry == nil {
		return false
	}

	b := tableEntry(*entry)

	addressValid := b.address.Addr().Is4() && b.address.IsValid()

	maskValid := b.broadcastDistributionMask[3] >= b.broadcastDistributionMask[2] &&
		b.broadcastDistributionMask[2] >= b.broadcastDistributionMask[1] &&
		b.broadcastDistributionMask[1] >= b.broadcastDistributionMask[0]

	return addressValid && maskValid
}

const (
	entryDataLen    = 10
	BdtEntryDataLen = entryDataLen
	FdtEntryDataLen = entryDataLen
)

// NewBdtEntry constructs a validated BdtEntry.
// address must be a valid IPv4 address-port pair.
// broadcastDistributionMask must be exactly 4 bytes.
func NewBdtEntry(address netip.AddrPort, broadcastDistributionMask net.IPMask) (*BdtEntry, error) {
	entry, err := newTableEntry[BdtEntry](address, broadcastDistributionMask)
	if err != nil {
		return nil, err
	}

	if !entry.Valid() {
		return nil, bacnet.NewValidationError("broadcast distribution mask", broadcastDistributionMask, ErrInvalidMask)
	}
	return entry, nil
}

type entryList[T tableEntryKinds] []T

func (l *entryList[T]) Decode(data []byte) error {
	if l == nil {
		return fmt.Errorf("cannot decode into nil list")
	}

	entries := make([]T, 0)
	for i := 0; i < len(data); i += BdtEntryDataLen {
		entryBytes := data[i : i+BdtEntryDataLen]
		var entry T
		err := decodeTableEntry(entryBytes, &entry)
		if err != nil {
			return fmt.Errorf("could not decode entry %d: %w", i, err)
		}

		entries = append(entries, entry)
	}

	*l = entries

	return nil
}

func (l *entryList[T]) Encode() ([]byte, error) {
	if l == nil {
		return nil, fmt.Errorf("cannot encode nil list")
	}

	out := make([]byte, 0)

	for i, entry := range *l {
		entryBytes, err := encodeTableEntry(&entry)
		if err != nil {
			return nil, fmt.Errorf("could not encode entry %d: %w", i, err)
		}

		out = append(out, entryBytes...)
	}

	return out, nil
}

func (l *entryList[T]) Valid() bool {
	if l == nil {
		return false
	}

	for _, entry := range *l {
		if !tableEntryValid[T](&entry) {
			return false
		}
	}

	return true
}

type BdtEntryList entryList[BdtEntry]

func (l *BdtEntryList) Decode(data []byte) error {
	if l == nil {
		return fmt.Errorf("cannot decode into nil list")
	}

	return (*entryList[BdtEntry])(l).Decode(data)
}

func (l *BdtEntryList) Encode() ([]byte, error) {
	return (*entryList[BdtEntry])(l).Encode()
}

func (l *BdtEntryList) Valid() bool {
	return (*entryList[BdtEntry])(l).Valid()
}

type FdtEntryList entryList[FdtEntry]

func (l *FdtEntryList) Decode(data []byte) error {
	return (*entryList[FdtEntry])(l).Decode(data)
}

func (l *FdtEntryList) Encode() ([]byte, error) {
	return (*entryList[FdtEntry])(l).Encode()
}

func (l *FdtEntryList) Valid() bool {
	return (*entryList[FdtEntry])(l).Valid()
}

type WriteBroadcastDistributionTable struct {
	header     BVLCHeader
	bdtEntries BdtEntryList
}

func (w *WriteBroadcastDistributionTable) BVLCFunctionType() BVLCFunctionType {
	return FunctionWriteBroadcastDistributionTable
}

func (w *WriteBroadcastDistributionTable) Valid() bool {
	if w == nil {
		return false
	}

	return w.header.Valid() && w.bdtEntries.Valid()
}

func (w *WriteBroadcastDistributionTable) Encode() ([]byte, error) {
	if w == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-write-broadcast-distribution-table")
	}

	headerBytes, err := w.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-write-broadcast-distribution-table: %w", err)
	}

	listBytes, err := w.bdtEntries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-write-broadcast-distribution-table: %w", err)
	}

	return append(headerBytes, listBytes...), nil
}

func (w *WriteBroadcastDistributionTable) Decode(data []byte) error {
	if len(data) < BVLCHeaderLen+BdtEntryDataLen { // cannot contain less than one entry
		return fmt.Errorf("invalid length for bvlc-write-broadcast-distribution-table")
	}

	res := WriteBroadcastDistributionTable{
		header:     BVLCHeader{},
		bdtEntries: make(BdtEntryList, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-write-broadcast-distribution-table header: %w", err)
	}

	err = res.bdtEntries.Decode(data[BVLCHeaderLen:])
	if err != nil {
		return fmt.Errorf("decode bvlc-write-broadcast-distribution-table %w", err)
	}

	if !res.Valid() {
		return fmt.Errorf("decoded invalid bvlc-write-broadcast-distribution-table")
	}

	*w = res

	return nil
}

// Entries returns a defensive copy of the BDT entries.
func (w *WriteBroadcastDistributionTable) Entries() []BdtEntry {
	out := make([]BdtEntry, len(w.bdtEntries))
	copy(out, w.bdtEntries)
	return out
}

// NewWriteBroadcastDistributionTable constructs a validated WriteBroadcastDistributionTable
// for BACnet/IP (IPv4). entries must contain at least one valid BdtEntry.
func NewWriteBroadcastDistributionTable(entries []BdtEntry) (*WriteBroadcastDistributionTable, error) {
	if len(entries) == 0 {
		return nil, bacnet.NewValidationError("entries", len(entries), ErrInvalidLength)
	}

	for i, e := range entries {
		if !e.Valid() {
			return nil, bacnet.NewValidationError(fmt.Sprintf("entries[%d]", i), e, ErrInvalidIPAddress)
		}
	}

	entriesCopy := make(BdtEntryList, len(entries))
	copy(entriesCopy, entries)
	totalLen := BVLCHeaderLen + len(entries)*BdtEntryDataLen

	return &WriteBroadcastDistributionTable{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionWriteBroadcastDistributionTable,
			BVLCLength:       BVLCLength(totalLen),
		},
		bdtEntries: entriesCopy,
	}, nil
}

type ReadBroadCastDistributionTable struct {
	header BVLCHeader
}

func NewReadBroadCastDistributionTable(length uint16) *ReadBroadCastDistributionTable {
	return &ReadBroadCastDistributionTable{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionReadBroadcastDistributionTable,
			BVLCLength:       BVLCLength(length),
		},
	}
}

func (r *ReadBroadCastDistributionTable) BVLCFunctionType() BVLCFunctionType {
	return FunctionReadBroadcastDistributionTable
}

func (r *ReadBroadCastDistributionTable) Valid() bool {
	if r == nil {
		return false
	}

	return r.header.Valid()
}

func (r *ReadBroadCastDistributionTable) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-read-broadcast-distribution-table")
	}

	out, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table: %w", err)
	}

	return out, nil
}

func (r *ReadBroadCastDistributionTable) Decode(data []byte) error {
	res := ReadBroadCastDistributionTable{}

	err := res.header.Decode(data)
	if err != nil {
		return fmt.Errorf("decode bvlc-read-broadcast-distribution-table: %w", err)
	}

	*r = res

	return nil
}

type ReadBroadcastDistributionTable struct {
	header  BVLCHeader
	entries BdtEntryList
}

func (r *ReadBroadcastDistributionTable) BVLCFunctionType() BVLCFunctionType {
	return FunctionReadBroadcastDistributionTable
}

func (r *ReadBroadcastDistributionTable) Valid() bool {
	if r == nil {
		return false
	}

	return r.header.Valid() && r.entries.Valid()
}

func (r *ReadBroadcastDistributionTable) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-read-broadcast-distribution-table")
	}

	headerBytes, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table: %w", err)
	}

	entryListBytes, err := r.entries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table: %w", err)
	}

	return append(headerBytes, entryListBytes...), nil
}

func (r *ReadBroadcastDistributionTable) Decode(data []byte) error {
	if len(data) < BVLCHeaderLen { // cannot contain header
		return fmt.Errorf("invalid length for bvlc-read-broadcast-distribution-table")
	}

	res := ReadBroadcastDistributionTable{
		header:  BVLCHeader{},
		entries: make([]BdtEntry, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-broadcast-distribution-table header: %w", err)
	}

	if len(data) > BVLCHeaderLen { // does not contain any entries, this is valid
		err = res.entries.Decode(data[BVLCHeaderLen:])
		if err != nil {
			return fmt.Errorf("decode bvlc-read-broadcast-distribution-table %w", err)
		}
	}

	*r = res

	return nil
}

func NewReadBroadcastDistributionTable(entries []BdtEntry) *ReadBroadcastDistributionTable {
	if entries == nil {
		entries = make([]BdtEntry, 0)
	}

	return &ReadBroadcastDistributionTable{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionReadBroadcastDistributionTable,
			BVLCLength:       BVLCLength(len(entries) * BdtEntryDataLen),
		},
		entries: entries,
	}
}

// BdtEntries returns a defensive copy of the BDT entries.
func (r *ReadBroadcastDistributionTable) BdtEntries() []BdtEntry {
	out := make([]BdtEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

type ReadBroadcastDistributionTableAck struct {
	header  BVLCHeader
	entries BdtEntryList
}

func (r *ReadBroadcastDistributionTableAck) BVLCFunctionType() BVLCFunctionType {
	return FunctionReadBroadcastDistributionTableAck
}

func (r *ReadBroadcastDistributionTableAck) Valid() bool {
	if r == nil {
		return false
	}

	return r.header.Valid() && r.entries.Valid()
}

func (r *ReadBroadcastDistributionTableAck) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-read-broadcast-distribution-table-ack")
	}

	headerBytes, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table-ack: %w", err)
	}

	entryListBytes, err := r.entries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table-ack: %w", err)
	}

	return append(headerBytes, entryListBytes...), nil
}

func (r *ReadBroadcastDistributionTableAck) Decode(data []byte) error {
	res := ReadBroadcastDistributionTableAck{
		header:  BVLCHeader{},
		entries: make([]BdtEntry, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-broadcast-distribution-table-ack: %w", err)
	}

	if len(data) > BVLCHeaderLen {
		err = res.entries.Decode(data[BVLCHeaderLen:])
		if err != nil {
			return fmt.Errorf("decode bvlc-read-broadcast-distribution-table %w", err)
		}
	}

	if !res.Valid() {
		return fmt.Errorf("decoded invalid bvlc-read-broadcast-distribution-table-ack")
	}

	*r = res
	return nil
}

// BdtEntries returns a defensive copy of the BDT entries carried in the ack.
func (r *ReadBroadcastDistributionTableAck) BdtEntries() []BdtEntry {
	out := make([]BdtEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// NewReadBroadcastDistributionTableAck constructs a validated
// ReadBroadcastDistributionTableAck for BACnet/IP (IPv4).
// entries may be empty (an empty BDT is valid per the standard).
func NewReadBroadcastDistributionTableAck(entries []BdtEntry) (*ReadBroadcastDistributionTableAck, error) {
	if entries == nil {
		entries = make([]BdtEntry, 0)
	}
	for i, e := range entries {
		if !e.Valid() {
			return nil, bacnet.NewValidationError(fmt.Sprintf("entries[%d]", i), e, ErrInvalidIPAddress)
		}
	}
	entriesCopy := make(BdtEntryList, len(entries))
	copy(entriesCopy, entries)
	totalLen := BVLCHeaderLen + len(entries)*BdtEntryDataLen
	return &ReadBroadcastDistributionTableAck{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionReadBroadcastDistributionTableAck,
			BVLCLength:       BVLCLength(totalLen),
		},
		entries: entriesCopy,
	}, nil
}

type ForwardedNpdu struct {
	header                          BVLCHeader
	addressOfOriginatingDevice      netip.AddrPort
	bacNetNpduFromOriginatingDevice []byte
}

func (f *ForwardedNpdu) BVLCFunctionType() BVLCFunctionType {
	return FunctionForwardedNPDU
}

func (f *ForwardedNpdu) Valid() bool {
	if f == nil {
		return false
	}

	return f.header.Valid() && f.addressOfOriginatingDevice.IsValid() && len(f.bacNetNpduFromOriginatingDevice) > 0
	//todo check if npdu has some proper definition somewhere in the standard, for now "there is data there" is enough though
}

func (f *ForwardedNpdu) Encode() ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-forwarded-npdu")
	}

	headerBytes, err := f.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-forwarded-npdu: %w", err)
	}

	addressBytes := encodeAddressPortIpV4(f.addressOfOriginatingDevice)

	return append(headerBytes, append(addressBytes, f.bacNetNpduFromOriginatingDevice...)...), nil
}

func (f *ForwardedNpdu) Decode(data []byte) error {
	if len(data) < BVLCHeaderLen+6 { // cannot contain header and address
		return fmt.Errorf("invalid length for bvlc-forwarded-npdu")
	}

	res := ForwardedNpdu{
		header:                          BVLCHeader{},
		addressOfOriginatingDevice:      netip.AddrPort{},
		bacNetNpduFromOriginatingDevice: make([]byte, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-forwarded-npdu: %w", err)
	}

	res.addressOfOriginatingDevice, err = decodeAddressPortIpV4(data[BVLCHeaderLen : BVLCHeaderLen+6])
	if err != nil {
		return fmt.Errorf("decode bvlc-forwarded-npdu address: %w", err)
	}

	res.bacNetNpduFromOriginatingDevice = cloneBytes(data[BVLCHeaderLen+6:])

	return nil
}

// OriginatingDeviceAddress returns the IP address and port of the originating device.
func (f *ForwardedNpdu) OriginatingDeviceAddress() netip.AddrPort {
	return f.addressOfOriginatingDevice
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (f *ForwardedNpdu) NPDUBytes() []byte {
	return cloneBytes(f.bacNetNpduFromOriginatingDevice)
}

// NewForwardedNpdu constructs a validated ForwardedNpdu for BACnet/IP (IPv4).
// originAddr must be a valid IPv4 address-port pair.
// npdu must be non-empty.
func NewForwardedNpdu(originAddr netip.AddrPort, npdu []byte) (*ForwardedNpdu, error) {
	if !originAddr.Addr().Is4() || !originAddr.IsValid() {
		return nil, bacnet.NewValidationError("origin address", originAddr, ErrInvalidIPAddress)
	}

	if len(npdu) == 0 {
		return nil, bacnet.NewValidationError("npdu", len(npdu), ErrInvalidLength)
	}

	totalLen := BVLCHeaderLen + 6 + len(npdu)
	if totalLen > 0xFFFF {
		return nil, bacnet.NewValidationError("length", totalLen, ErrInvalidLength)
	}

	return &ForwardedNpdu{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionForwardedNPDU,
			BVLCLength:       BVLCLength(totalLen),
		},
		addressOfOriginatingDevice:      originAddr,
		bacNetNpduFromOriginatingDevice: cloneBytes(npdu),
	}, nil
}

func encodeAddressPortIpV4(address netip.AddrPort) []byte {
	out := make([]byte, 0, 6)

	copy(out[0:4], address.Addr().AsSlice())
	binary.BigEndian.PutUint16(out[4:], address.Port())

	return out
}

func decodeAddressPortIpV4(data []byte) (netip.AddrPort, error) {
	if len(data) != 6 {
		return netip.AddrPort{}, fmt.Errorf("invalid length for bvlc-forwarded-npdu")
	}

	addr := netip.AddrFrom4([4]byte{data[0], data[1], data[2], data[3]})
	port := binary.BigEndian.Uint16(data[4:6])

	return netip.AddrPortFrom(addr, port), nil
}

// TTL is a time to live in seconds
type TTL uint16

func (t TTL) ToDuration() time.Duration {
	return time.Duration(t) * time.Second
}

type RegisterForeignDevice struct {
	header BVLCHeader
	ttl    TTL
}

func (r *RegisterForeignDevice) BVLCFunctionType() BVLCFunctionType {
	return FunctionRegisterForeignDevice
}

func (r *RegisterForeignDevice) Valid() bool {
	return r.header.Valid() && r.ttl != 0
}

func (r *RegisterForeignDevice) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-register-foreign-device")
	}

	headerBytes, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-register-foreign-device: %w", err)
	}

	out := make([]byte, BVLCHeaderLen+2)
	copy(out[0:BVLCHeaderLen], headerBytes)

	binary.BigEndian.PutUint16(out[BVLCHeaderLen:], uint16(r.ttl))

	return out, nil
}

func (r *RegisterForeignDevice) Decode(data []byte) error {
	if r == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) != BVLCHeaderLen+2 {
		return fmt.Errorf("invalid length for bvlc-register-foreign-device")
	}

	res := RegisterForeignDevice{
		header: BVLCHeader{},
		ttl:    TTL(0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-register-foreign-device: %w", err)
	}

	res.ttl = TTL(binary.BigEndian.Uint16(data[BVLCHeaderLen:]))

	*r = res

	return nil
}

// TTL returns the time-to-live value of the foreign device registration.
func (r *RegisterForeignDevice) TTL() TTL {
	return r.ttl
}

// NewRegisterForeignDevice constructs a validated RegisterForeignDevice for BACnet/IP (IPv4).
// ttl must be non-zero.
func NewRegisterForeignDevice(ttl TTL) (*RegisterForeignDevice, error) {
	if ttl == 0 {
		return nil, bacnet.NewValidationError("ttl", ttl, ErrInvalidTTL)
	}

	const frameLen = BVLCHeaderLen + 2

	return &RegisterForeignDevice{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionRegisterForeignDevice,
			BVLCLength:       BVLCLength(frameLen),
		},
		ttl: ttl,
	}, nil
}

type ReadForeignDeviceTable struct {
	header BVLCHeader
}

func (r *ReadForeignDeviceTable) BVLCFunctionType() BVLCFunctionType {
	return FunctionReadForeignDeviceTable
}

func (r *ReadForeignDeviceTable) Valid() bool {
	return r.header.Valid()
}

func (r *ReadForeignDeviceTable) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-read-foreign-device-table")
	}

	return r.header.Encode()
}

func (r *ReadForeignDeviceTable) Decode(data []byte) error {
	if r == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	res := ReadForeignDeviceTable{
		header: BVLCHeader{},
	}

	err := res.header.Decode(data[:])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-foreign-device-table: %w", err)
	}

	*r = res

	return nil
}

// NewReadForeignDeviceTable constructs a ReadForeignDeviceTable request for BACnet/IP (IPv4).
func NewReadForeignDeviceTable() *ReadForeignDeviceTable {
	return &ReadForeignDeviceTable{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionReadForeignDeviceTable,
			BVLCLength:       BVLCHeaderLen,
		},
	}
}

type ReadForeignDeviceTableAck struct {
	header  BVLCHeader
	entries FdtEntryList
}

func (r *ReadForeignDeviceTableAck) BVLCFunctionType() BVLCFunctionType {
	return FunctionReadForeignDeviceTableAck
}

func (r *ReadForeignDeviceTableAck) Valid() bool {
	if r == nil {
		return false
	}

	return r.header.Valid() && r.entries.Valid()
}

func (r *ReadForeignDeviceTableAck) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-read-foreign-device-table-ack")
	}

	headerBytes, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-foreign-device-table-ack: %w", err)
	}

	listBytes, err := r.entries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-foreign-device-table-ack: %w", err)
	}

	return append(headerBytes, listBytes...), nil
}

func (r *ReadForeignDeviceTableAck) Decode(data []byte) error {
	if r == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	res := ReadForeignDeviceTableAck{
		header:  BVLCHeader{},
		entries: make(FdtEntryList, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-foreign-device-table-ack: %w", err)
	}

	err = res.entries.Decode(data[BVLCHeaderLen:])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-foreign-device-table-ack: %w", err)
	}

	*r = res

	return nil
}

// Entries returns a defensive copy of the FDT entries carried in the ack.
func (r *ReadForeignDeviceTableAck) Entries() []FdtEntry {
	out := make([]FdtEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// NewReadForeignDeviceTableAck constructs a validated ReadForeignDeviceTableAck
// for BACnet/IP (IPv4). entries may be empty.
func NewReadForeignDeviceTableAck(entries []FdtEntry) (*ReadForeignDeviceTableAck, error) {
	if entries == nil {
		entries = make([]FdtEntry, 0)
	}
	for i, e := range entries {
		if !e.Valid() {
			return nil, bacnet.NewValidationError(fmt.Sprintf("entries[%d]", i), e, ErrInvalidIPAddress)
		}
	}
	entriesCopy := make(FdtEntryList, len(entries))
	copy(entriesCopy, entries)
	totalLen := BVLCHeaderLen + len(entries)*FdtEntryDataLen
	return &ReadForeignDeviceTableAck{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionReadForeignDeviceTableAck,
			BVLCLength:       BVLCLength(totalLen),
		},
		entries: entriesCopy,
	}, nil
}

type DeleteForeignDeviceTableEntry struct {
	header BVLCHeader
	entry  BdtEntry
}

func (d *DeleteForeignDeviceTableEntry) BVLCFunctionType() BVLCFunctionType {
	return FunctionDeleteForeignDeviceTableEntry
}

func (d *DeleteForeignDeviceTableEntry) Valid() bool {
	return d.header.Valid() && d.entry.Valid()
}

func (d *DeleteForeignDeviceTableEntry) Encode() ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("cannot encode nil delete-foreign-device-table-entry")
	}

	headerBytes, err := d.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode delete-foreign-device-table-entry: %w", err)
	}

	entryBytes, err := d.entry.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode delete-foreign-device-table-entry: %w", err)
	}

	return append(headerBytes, entryBytes...), nil
}

func (d *DeleteForeignDeviceTableEntry) Decode(data []byte) error {
	if d == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) != BVLCHeaderLen+BdtEntryDataLen {
		return fmt.Errorf("invalid length for delete-foreign-device-table-entry")
	}

	res := DeleteForeignDeviceTableEntry{
		header: BVLCHeader{},
		entry:  BdtEntry{},
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode delete-foreign-device-table-entry header: %w", err)
	}

	err = res.entry.Decode(data[BVLCHeaderLen:])
	if err != nil {
		return fmt.Errorf("decode delete-foreign-device-table-entry entry: %w", err)
	}

	if !res.Valid() {
		return fmt.Errorf("decoded delete-foreign-device-table-entry invalid")
	}

	*d = res

	return nil
}

// BdtEntry returns a copy of the FDT entry to be deleted.
func (d *DeleteForeignDeviceTableEntry) BdtEntry() BdtEntry {
	return d.entry
}

// NewDeleteForeignDeviceTableEntry constructs a validated DeleteForeignDeviceTableEntry
// for BACnet/IP (IPv4). entry must be valid.
func NewDeleteForeignDeviceTableEntry(entry BdtEntry) (*DeleteForeignDeviceTableEntry, error) {
	if !entry.Valid() {
		return nil, bacnet.NewValidationError("entry", entry, ErrInvalidIPAddress)
	}
	const frameLen = BVLCHeaderLen + BdtEntryDataLen
	return &DeleteForeignDeviceTableEntry{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionDeleteForeignDeviceTableEntry,
			BVLCLength:       BVLCLength(frameLen),
		},
		entry: entry,
	}, nil
}

type DistributeBroadcastToNetwork struct {
	header                          BVLCHeader
	bacnetNpduFromOriginatingDevice []byte
}

func (d *DistributeBroadcastToNetwork) BVLCFunctionType() BVLCFunctionType {
	return FunctionDistributeBroadcastToNetwork
}

func (d *DistributeBroadcastToNetwork) Valid() bool {
	if d == nil {
		return false
	}
	return d.header.Valid() && d.bacnetNpduFromOriginatingDevice != nil //TODO check npdu format
}

func (d *DistributeBroadcastToNetwork) Encode() ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-distribute-broadcast-to-network")
	}

	headerBytes, err := d.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-distribute-broadcast-to-network: %w", err)
	}

	return append(headerBytes, d.bacnetNpduFromOriginatingDevice...), nil
}

func (d *DistributeBroadcastToNetwork) Decode(data []byte) error {
	if d == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) <= BVLCHeaderLen {
		return fmt.Errorf("invalid length for distribute-broadcast-to-network")
	}

	res := DistributeBroadcastToNetwork{
		header:                          BVLCHeader{},
		bacnetNpduFromOriginatingDevice: make([]byte, 0, len(data)-BVLCHeaderLen),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-distribute-broadcast-to-network: %w", err)
	}

	copy(res.bacnetNpduFromOriginatingDevice, data[BVLCHeaderLen:])

	*d = res

	return nil
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (d *DistributeBroadcastToNetwork) NPDUBytes() []byte {
	return cloneBytes(d.bacnetNpduFromOriginatingDevice)
}

// NewDistributeBroadcastToNetwork constructs a validated DistributeBroadcastToNetwork.
// frameType must be BVLCTypeBACnetIP (0x81) or BVLCTypeBACnetIP6 (0x82).
// npdu must be non-empty.
func NewDistributeBroadcastToNetwork(frameType BVLCType, npdu []byte) (*DistributeBroadcastToNetwork, error) {
	if !frameType.Valid() {
		return nil, bacnet.NewValidationError("frame type", frameType, ErrInvalidBVLCType)
	}
	if len(npdu) == 0 {
		return nil, bacnet.NewValidationError("npdu", len(npdu), ErrInvalidLength)
	}
	totalLen := BVLCHeaderLen + len(npdu)
	if totalLen > 0xFFFF {
		return nil, bacnet.NewValidationError("length", totalLen, ErrInvalidLength)
	}
	return &DistributeBroadcastToNetwork{
		header: BVLCHeader{
			BVLCType:         frameType,
			BVLCFunctionType: FunctionDistributeBroadcastToNetwork,
			BVLCLength:       BVLCLength(totalLen),
		},
		bacnetNpduFromOriginatingDevice: cloneBytes(npdu),
	}, nil
}

// OriginalUnicastNpdu is a BVLC Original-Unicast-NPDU message (Annex J, function 0x0A).
// It carries a BACnet NPDU addressed to a single peer.
type OriginalUnicastNpdu struct {
	header     BVLCHeader
	bacnetNpdu []byte
}

// NewOriginalUnicastNpdu constructs a validated OriginalUnicastNpdu.
// frameType must be BVLCTypeBACnetIP (0x81) or BVLCTypeBACnetIP6 (0x82).
// npdu must be non-empty.
func NewOriginalUnicastNpdu(frameType BVLCType, npdu []byte) (*OriginalUnicastNpdu, error) {
	if !frameType.Valid() {
		return nil, bacnet.NewValidationError("frame type", frameType, ErrInvalidBVLCType)
	}
	if len(npdu) == 0 {
		return nil, bacnet.NewValidationError("npdu", len(npdu), ErrInvalidLength)
	}
	totalLen := BVLCHeaderLen + len(npdu)
	if totalLen > 0xFFFF {
		return nil, bacnet.NewValidationError("length", totalLen, ErrInvalidLength)
	}
	l, err := NewBVLCLength(totalLen)
	if err != nil {
		return nil, err
	}
	return &OriginalUnicastNpdu{
		header: BVLCHeader{
			BVLCType:         frameType,
			BVLCFunctionType: FunctionOriginalUnicastNPDU,
			BVLCLength:       l,
		},
		bacnetNpdu: cloneBytes(npdu),
	}, nil
}

// BVLCFunctionType returns FunctionOriginalUnicastNPDU.
func (o *OriginalUnicastNpdu) BVLCFunctionType() BVLCFunctionType {
	return FunctionOriginalUnicastNPDU
}

// Valid reports whether the struct holds a consistent, encodable state.
func (o *OriginalUnicastNpdu) Valid() bool {
	if o == nil {
		return false
	}
	return o.header.Valid() &&
		o.header.BVLCFunctionType == FunctionOriginalUnicastNPDU &&
		len(o.bacnetNpdu) > 0
}

// Encode serializes the message to wire bytes (BVLC header + NPDU).
func (o *OriginalUnicastNpdu) Encode() ([]byte, error) {
	if o == nil {
		return nil, fmt.Errorf("cannot encode nil original-unicast-npdu")
	}
	if !o.Valid() {
		return nil, fmt.Errorf("invalid original-unicast-npdu")
	}
	headerBytes, err := o.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode original-unicast-npdu: %w", err)
	}

	return append(headerBytes, o.bacnetNpdu...), nil
}

// Decode parses wire bytes into the receiver.
// data must begin with the 4-byte BVLC header followed by at least one NPDU byte.
func (o *OriginalUnicastNpdu) Decode(data []byte) error {
	if o == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}
	if len(data) <= BVLCHeaderLen {
		return fmt.Errorf("invalid length for original-unicast-npdu")
	}
	res := OriginalUnicastNpdu{}
	if err := res.header.Decode(data[:BVLCHeaderLen]); err != nil {
		return fmt.Errorf("decode original-unicast-npdu header: %w", err)
	}
	if res.header.BVLCFunctionType != FunctionOriginalUnicastNPDU {
		return fmt.Errorf("invalid function type for original-unicast-npdu: %s", res.header.BVLCFunctionType)
	}
	res.bacnetNpdu = cloneBytes(data[BVLCHeaderLen:])
	*o = res
	return nil
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (o *OriginalUnicastNpdu) NPDUBytes() []byte {
	return cloneBytes(o.bacnetNpdu)
}

// OriginalBroadcastNpdu is a BVLC Original-Broadcast-NPDU message (Annex J, function 0x0B).
// It carries a BACnet NPDU that is to be broadcast on the local IP subnet.
type OriginalBroadcastNpdu struct {
	header     BVLCHeader
	bacnetNpdu []byte
}

func NewOriginalBroadcastNpdu(frameType BVLCType, npdu []byte) (*OriginalBroadcastNpdu, error) {
	if !frameType.Valid() {
		return nil, bacnet.NewValidationError("frame type", frameType, ErrInvalidBVLCType)
	}
	if len(npdu) == 0 {
		return nil, bacnet.NewValidationError("npdu", len(npdu), ErrInvalidLength)
	}
	totalLen := BVLCHeaderLen + len(npdu)
	if totalLen > 0xFFFF {
		return nil, bacnet.NewValidationError("length", totalLen, ErrInvalidLength)
	}
	l, err := NewBVLCLength(totalLen)
	if err != nil {
		return nil, err
	}
	return &OriginalBroadcastNpdu{
		header: BVLCHeader{
			BVLCType:         frameType,
			BVLCFunctionType: FunctionOriginalBroadcastNPDU,
			BVLCLength:       l,
		},
		bacnetNpdu: cloneBytes(npdu),
	}, nil
}

func (o *OriginalBroadcastNpdu) BVLCFunctionType() BVLCFunctionType {
	return FunctionOriginalBroadcastNPDU
}

func (o *OriginalBroadcastNpdu) Valid() bool {
	if o == nil {
		return false
	}
	return o.header.Valid() &&
		o.header.BVLCFunctionType == FunctionOriginalBroadcastNPDU &&
		len(o.bacnetNpdu) > 0
}

func (o *OriginalBroadcastNpdu) Encode() ([]byte, error) {
	if o == nil {
		return nil, fmt.Errorf("cannot encode nil original-broadcast-npdu")
	}
	if !o.Valid() {
		return nil, fmt.Errorf("invalid original-broadcast-npdu")
	}
	out, err := o.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode original-broadcast-npdu: %w", err)
	}
	copy(out[BVLCHeaderLen:], o.bacnetNpdu)
	return out, nil
}

// Decode parses wire bytes into the receiver.
// data must begin with the 4-byte BVLC header followed by at least one NPDU byte.
func (o *OriginalBroadcastNpdu) Decode(data []byte) error {
	if o == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}
	if len(data) <= BVLCHeaderLen {
		return fmt.Errorf("invalid length for original-broadcast-npdu")
	}
	res := OriginalBroadcastNpdu{}
	if err := res.header.Decode(data[:BVLCHeaderLen]); err != nil {
		return fmt.Errorf("decode original-broadcast-npdu header: %w", err)
	}
	if res.header.BVLCFunctionType != FunctionOriginalBroadcastNPDU {
		return fmt.Errorf("invalid function type for original-broadcast-npdu: %s", res.header.BVLCFunctionType)
	}
	res.bacnetNpdu = cloneBytes(data[BVLCHeaderLen:])
	*o = res
	return nil
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (o *OriginalBroadcastNpdu) NPDUBytes() []byte {
	return cloneBytes(o.bacnetNpdu)
}

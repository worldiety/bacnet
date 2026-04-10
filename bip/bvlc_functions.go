package bip

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"net/netip"
	"time"

	"go.wdy.de/bacnet"
	"go.wdy.de/bacnet/internal/util"
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

type FdtEntry struct {
	address       netip.AddrPort
	registeredTtl TTL
	remainingTtl  TTL
}

func (f *FdtEntry) Valid() bool {
	if f == nil {
		return false
	}

	remainingTtlValid := f.remainingTtl <= TTL(min(int(f.registeredTtl)+30, math.MaxUint16))

	return f.address.Addr().Is4() && f.address.IsValid() &&
		f.registeredTtl > 0 && remainingTtlValid
}

func (f *FdtEntry) Encode() ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("nil bvlc-entry")
	}

	out := make([]byte, FdtEntryDataLen)

	if !f.address.Addr().Is4() { //fdt entries require ipv4, should be guaranteed by constructor, check here anyway
		return nil, fmt.Errorf("invalid bvlc-address, expected IPv4")
	}

	copy(out[0:6], encodeAddressPortIpV4(f.address))

	binary.BigEndian.PutUint16(out[6:8], uint16(f.registeredTtl))
	binary.BigEndian.PutUint16(out[8:10], uint16(f.remainingTtl))

	return out, nil
}

func (f *FdtEntry) Decode(data []byte) error {
	if f == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) != FdtEntryDataLen {
		return fmt.Errorf("invalid length for fdt entry: %d", len(data))
	}

	address, err := decodeAddressPortIpV4(data[0:6])
	if err != nil {
		return fmt.Errorf("invalid ip in fdt entry: %w", err)
	}

	res := FdtEntry{
		address:       address,
		registeredTtl: 0,
		remainingTtl:  0,
	}

	res.registeredTtl = TTL(binary.BigEndian.Uint16(data[6:8]))
	res.remainingTtl = TTL(binary.BigEndian.Uint16(data[8:10]))

	if !res.Valid() {
		return fmt.Errorf("decoded invalid fdt entry")
	}

	*f = res

	return nil
}

// Address returns the IP address and port of the foreign device.
func (f *FdtEntry) Address() netip.AddrPort {
	return f.address
}

func (f *FdtEntry) RemainingTtl() TTL {
	return f.remainingTtl
}

func (f *FdtEntry) RegisteredTtl() TTL {
	return f.registeredTtl
}

// NewFdtEntry constructs a validated FdtEntry.
// address must be a valid IPv4 address-port pair.
func NewFdtEntry(address netip.AddrPort, ttl TTL) (*FdtEntry, error) {
	if !address.Addr().Is4() || !address.IsValid() {
		return nil, bacnet.NewValidationError("address", address, ErrInvalidIPAddress)
	}

	if ttl == 0 {
		return nil, bacnet.NewValidationError("ttl", ttl, ErrInvalidTTL)
	}

	remainingTtl := int(ttl) + 30

	if remainingTtl > math.MaxUint16 {
		remainingTtl = math.MaxUint16
	}

	entry := &FdtEntry{
		address:       address,
		registeredTtl: ttl,
		remainingTtl:  TTL(remainingTtl),
	}

	return entry, nil
}

type BdtEntry struct {
	address                   netip.AddrPort
	broadcastDistributionMask net.IPMask
}

func (b *BdtEntry) Valid() bool {
	if b == nil || len(b.broadcastDistributionMask) != net.IPv4len {
		return false
	}

	addressValid := b.address.Addr().Is4() && b.address.IsValid()

	maskValid := b.broadcastDistributionMask[0] >= b.broadcastDistributionMask[1] &&
		b.broadcastDistributionMask[1] >= b.broadcastDistributionMask[2] &&
		b.broadcastDistributionMask[2] >= b.broadcastDistributionMask[3]

	return addressValid && maskValid
}

func (b *BdtEntry) Encode() ([]byte, error) {
	if b == nil {
		return nil, fmt.Errorf("cannot encode nil pointer")
	}

	out := make([]byte, BdtEntryDataLen)

	if !b.address.Addr().Is4() { //bd entries require ipv4, should be guaranteed by constructor, check here anyway
		return nil, fmt.Errorf("invalid bvlc-address, expected IPv4")
	}

	copy(out[0:6], encodeAddressPortIpV4(b.address))

	copy(out[6:10], b.broadcastDistributionMask)

	return out, nil
}

func (b *BdtEntry) Decode(data []byte) error {
	if b == nil {
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

	entry := BdtEntry{
		address:                   address,
		broadcastDistributionMask: mask,
	}

	if !entry.Valid() {
		return fmt.Errorf("invalid bdt entry mask") // ip and port are valid, invalidity must be caused by mask
	}

	*b = entry

	return nil
}

// Address returns the IP address and port of the BDT peer.
func (b *BdtEntry) Address() netip.AddrPort {
	return b.address
}

// BroadcastDistributionMask returns a defensive copy of the subnet broadcast mask.
func (b *BdtEntry) BroadcastDistributionMask() net.IPMask {
	return net.IPMask(util.CloneBytes(b.broadcastDistributionMask))
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
	if broadcastDistributionMask == nil || len(broadcastDistributionMask) != net.IPv4len {
		return nil, bacnet.NewValidationError("broadcast distribution mask", broadcastDistributionMask, ErrInvalidMask)
	}

	entry := BdtEntry{
		address:                   address,
		broadcastDistributionMask: util.CloneBytes(broadcastDistributionMask),
	}

	if !entry.Valid() {
		return nil, bacnet.NewValidationError("broadcast distribution mask", broadcastDistributionMask, ErrInvalidMask)
	}

	return &entry, nil
}

type BdtEntryList []BdtEntry

func (l *BdtEntryList) Decode(data []byte) error {
	if l == nil {
		return fmt.Errorf("cannot decode into nil list")
	}

	if len(data)%BdtEntryDataLen != 0 {
		return fmt.Errorf("invalid length for bdt entry: %d", len(data))
	}

	list := make([]BdtEntry, 0)

	for i := 0; i < len(data); i += BdtEntryDataLen {
		entry := BdtEntry{}
		err := entry.Decode(data[i : i+BdtEntryDataLen])
		if err != nil {
			return fmt.Errorf("invalid bdt entry %d: %w", i, err)
		}

		list = append(list, entry)
	}

	*l = list

	return nil
}

func (l *BdtEntryList) Encode() ([]byte, error) {
	if l == nil {
		return nil, fmt.Errorf("cannot encode into nil list")
	}

	out := make([]byte, 0, len(*l)*BdtEntryDataLen)

	for i, entry := range *l {
		entryBytes, err := entry.Encode()
		if err != nil {
			return nil, fmt.Errorf("invalid bdt entry %d: %w", i, err)
		}

		out = append(out, entryBytes...)
	}

	return out, nil
}

func (l *BdtEntryList) Valid() bool {
	if l == nil {
		return false
	}

	v := true
	for _, entry := range *l {
		v = v && entry.Valid()
	}

	return v
}

type FdtEntryList []FdtEntry

func (l *FdtEntryList) Decode(data []byte) error {
	if l == nil {
		return fmt.Errorf("cannot decode into nil list")
	}

	if len(data)%FdtEntryDataLen != 0 {
		return fmt.Errorf("invalid length for fdt entry list: %d", len(data))
	}

	list := make([]FdtEntry, 0)

	for i := 0; i < len(data); i += FdtEntryDataLen {
		entry := FdtEntry{}
		err := entry.Decode(data[i : i+FdtEntryDataLen])
		if err != nil {
			return fmt.Errorf("invalid fdt entry %d: %w", i, err)
		}

		list = append(list, entry)
	}

	*l = list

	return nil
}

func (l *FdtEntryList) Encode() ([]byte, error) {
	if l == nil {
		return nil, fmt.Errorf("cannot encode into nil list")
	}

	out := make([]byte, 0, len(*l)*FdtEntryDataLen)
	for i, entry := range *l {
		entryBytes, err := entry.Encode()
		if err != nil {
			return nil, fmt.Errorf("invalid fdt entry %d: %w", i, err)
		}

		out = append(out, entryBytes...)
	}

	return out, nil
}

func (l *FdtEntryList) Valid() bool {
	if l == nil {
		return false
	}

	v := true

	for _, entry := range *l {
		v = v && entry.Valid()
	}

	return v
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

	out, err := w.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-write-broadcast-distribution-table: %w", err)
	}

	listBytes, err := w.bdtEntries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-write-broadcast-distribution-table: %w", err)
	}

	if copy(out[BVLCHeaderLen:], listBytes) != len(listBytes) {
		return nil, fmt.Errorf("failed to encode bvlc-write-broadcast-distribution-table")
	}

	return out, nil
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

	if res.header.BVLCFunctionType != FunctionWriteBroadcastDistributionTable {
		return fmt.Errorf("invalid function type, expected bvlc-write-broadcast-distribution-table")
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

type ReadBroadcastDistributionTable struct {
	header BVLCHeader
}

func NewReadBroadcastDistributionTable() *ReadBroadcastDistributionTable {
	return &ReadBroadcastDistributionTable{
		header: BVLCHeader{
			BVLCType:         BVLCTypeBACnetIP,
			BVLCFunctionType: FunctionReadBroadcastDistributionTable,
			BVLCLength:       BVLCLength(BVLCHeaderLen),
		},
	}
}

func (r *ReadBroadcastDistributionTable) BVLCFunctionType() BVLCFunctionType {
	return FunctionReadBroadcastDistributionTable
}

func (r *ReadBroadcastDistributionTable) Valid() bool {
	if r == nil {
		return false
	}

	return r.header.Valid()
}

func (r *ReadBroadcastDistributionTable) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-read-broadcast-distribution-table")
	}

	out, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table: %w", err)
	}

	return out, nil
}

func (r *ReadBroadcastDistributionTable) Decode(data []byte) error {
	if len(data) != BVLCHeaderLen {
		return fmt.Errorf("invalid length for bvlc-read-broadcast-distribution-table")
	}

	res := ReadBroadcastDistributionTable{}

	err := res.header.Decode(data)
	if err != nil {
		return fmt.Errorf("decode bvlc-read-broadcast-distribution-table: %w", err)
	}

	if res.header.BVLCFunctionType != FunctionReadBroadcastDistributionTable {
		return fmt.Errorf("invalid function type, expected bvlc-read-broadcast-distribution-table")
	}

	*r = res

	return nil
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

	out, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table-ack: %w", err)
	}

	entryListBytes, err := r.entries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-broadcast-distribution-table-ack: %w", err)
	}

	if copy(out[BVLCHeaderLen:], entryListBytes) != len(entryListBytes) {
		return nil, fmt.Errorf("failed to encode bvlc-read-broadcast-distribution-table-ack")
	}

	return out, nil
}

func (r *ReadBroadcastDistributionTableAck) Decode(data []byte) error {
	if len(data) < BVLCHeaderLen {
		return fmt.Errorf("invalid length for bvlc-read-broadcast-distribution-table-ack")
	}

	res := ReadBroadcastDistributionTableAck{
		header:  BVLCHeader{},
		entries: make([]BdtEntry, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-broadcast-distribution-table-ack: %w", err)
	}

	if res.header.BVLCFunctionType != FunctionReadBroadcastDistributionTableAck {
		return fmt.Errorf("invalid function type, expected bvlc-read-broadcast-distribution-table-ack")
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

	out, err := f.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-forwarded-npdu: %w", err)
	}

	npduBytes := append(encodeAddressPortIpV4(f.addressOfOriginatingDevice), f.bacNetNpduFromOriginatingDevice...)

	if copy(out[BVLCHeaderLen:], npduBytes) != len(npduBytes) {
		return nil, fmt.Errorf("failed to encode bvlc-forwarded-npdu")
	}

	return out, nil
}

func (f *ForwardedNpdu) Decode(data []byte) error {
	if len(data) <= BVLCHeaderLen+6 { // cannot contain header and address
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

	if res.header.BVLCFunctionType != FunctionForwardedNPDU {
		return fmt.Errorf("invalid function type, expected bvlc-forwarded-npdu")
	}

	res.addressOfOriginatingDevice, err = decodeAddressPortIpV4(data[BVLCHeaderLen : BVLCHeaderLen+6])
	if err != nil {
		return fmt.Errorf("decode bvlc-forwarded-npdu address: %w", err)
	}

	res.bacNetNpduFromOriginatingDevice = util.CloneBytes(data[BVLCHeaderLen+6:])

	*f = res

	return nil
}

// OriginatingDeviceAddress returns the IP address and port of the originating device.
func (f *ForwardedNpdu) OriginatingDeviceAddress() netip.AddrPort {
	return f.addressOfOriginatingDevice
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (f *ForwardedNpdu) NPDUBytes() []byte {
	return util.CloneBytes(f.bacNetNpduFromOriginatingDevice)
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
		bacNetNpduFromOriginatingDevice: util.CloneBytes(npdu),
	}, nil
}

func encodeAddressPortIpV4(address netip.AddrPort) []byte {
	out := make([]byte, 6)

	ip4 := address.Addr().As4()

	copy(out[0:4], ip4[:])
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
	return r != nil && r.header.Valid() && r.ttl != 0
}

func (r *RegisterForeignDevice) Encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-register-foreign-device")
	}

	out, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-register-foreign-device: %w", err)
	}

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

	if res.header.BVLCFunctionType != FunctionRegisterForeignDevice {
		return fmt.Errorf("invalid bvlc function type, expected register-foreign-device")
	}

	res.ttl = TTL(binary.BigEndian.Uint16(data[BVLCHeaderLen:]))

	if !res.Valid() {
		return fmt.Errorf("decoded invalid bvlc-register-foreign-device")
	}

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
	return r != nil && r.header.Valid()
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

	if res.header.BVLCFunctionType != FunctionReadForeignDeviceTable {
		return fmt.Errorf("invalid bvlc function type, expected read-foreign-device-table")
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

	out, err := r.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-foreign-device-table-ack: %w", err)
	}

	listBytes, err := r.entries.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-read-foreign-device-table-ack: %w", err)
	}

	if copy(out[BVLCHeaderLen:], listBytes) != len(listBytes) {
		return nil, fmt.Errorf("failed to encode bvlc-read-foreign-device-table-ack")
	}

	return out, nil
}

func (r *ReadForeignDeviceTableAck) Decode(data []byte) error {
	if r == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) < BVLCHeaderLen {
		return fmt.Errorf("invalid length for bvlc-read-foreign-device-table-ack")
	}

	res := ReadForeignDeviceTableAck{
		header:  BVLCHeader{},
		entries: make(FdtEntryList, 0),
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-read-foreign-device-table-ack: %w", err)
	}

	if res.header.BVLCFunctionType != FunctionReadForeignDeviceTableAck {
		return fmt.Errorf("invalid bvlc function type, expected read-foreign-device-table-ack")
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
	entry  FdtEntry
}

func (d *DeleteForeignDeviceTableEntry) BVLCFunctionType() BVLCFunctionType {
	return FunctionDeleteForeignDeviceTableEntry
}

func (d *DeleteForeignDeviceTableEntry) Valid() bool {
	return d != nil && d.header.Valid() && d.entry.Valid()
}

func (d *DeleteForeignDeviceTableEntry) Encode() ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("cannot encode nil delete-foreign-device-table-entry")
	}

	out, err := d.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode delete-foreign-device-table-entry: %w", err)
	}

	entryBytes, err := d.entry.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode delete-foreign-device-table-entry: %w", err)
	}

	if copy(out[BVLCHeaderLen:], entryBytes) != len(entryBytes) {
		return nil, fmt.Errorf("could not encode delete-foreign-device-table-entry")
	}

	return out, nil
}

func (d *DeleteForeignDeviceTableEntry) Decode(data []byte) error {
	if d == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) != BVLCHeaderLen+entryDataLen {
		return fmt.Errorf("invalid length for delete-foreign-device-table-entry")
	}

	res := DeleteForeignDeviceTableEntry{
		header: BVLCHeader{},
		entry:  FdtEntry{},
	}

	err := res.header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode delete-foreign-device-table-entry header: %w", err)
	}

	if res.header.BVLCFunctionType != FunctionDeleteForeignDeviceTableEntry {
		return fmt.Errorf("invalid bvlc function type, expected delete-foreign-device-table-entry")
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

// FdtEntry returns a copy of the FDT entry to be deleted.
func (d *DeleteForeignDeviceTableEntry) FdtEntry() FdtEntry {
	return d.entry
}

// NewDeleteForeignDeviceTableEntry constructs a validated DeleteForeignDeviceTableEntry
// for BACnet/IP (IPv4). entry must be valid.
func NewDeleteForeignDeviceTableEntry(entry FdtEntry) (*DeleteForeignDeviceTableEntry, error) {
	if !entry.Valid() {
		return nil, bacnet.NewValidationError("entry", entry, ErrInvalidIPAddress)
	}
	const frameLen = BVLCHeaderLen + entryDataLen
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

	return d.header.Valid() && len(d.bacnetNpduFromOriginatingDevice) > 0 //TODO check npdu format
}

func (d *DistributeBroadcastToNetwork) Encode() ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-distribute-broadcast-to-network")
	}

	out, err := d.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-distribute-broadcast-to-network: %w", err)
	}

	if copy(out[BVLCHeaderLen:], d.bacnetNpduFromOriginatingDevice) != len(d.bacnetNpduFromOriginatingDevice) {
		return nil, fmt.Errorf("failed to encode bvlc-distribute-broadcast-to-network")
	}

	return out, nil
}

func (d *DistributeBroadcastToNetwork) Decode(data []byte) error {
	if d == nil {
		return fmt.Errorf("cannot decode into nil pointer")
	}

	if len(data) <= BVLCHeaderLen {
		return fmt.Errorf("invalid length for distribute-broadcast-to-network")
	}

	header := BVLCHeader{}

	err := header.Decode(data[:BVLCHeaderLen])
	if err != nil {
		return fmt.Errorf("decode bvlc-distribute-broadcast-to-network: %w", err)
	}

	if header.BVLCFunctionType != FunctionDistributeBroadcastToNetwork {
		return fmt.Errorf("invalid bvlc function type, expected DistributeBroadcastToNetwork")
	}

	*d = DistributeBroadcastToNetwork{
		header:                          header,
		bacnetNpduFromOriginatingDevice: util.CloneBytes(data[BVLCHeaderLen:]),
	}

	return nil
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (d *DistributeBroadcastToNetwork) NPDUBytes() []byte {
	return util.CloneBytes(d.bacnetNpduFromOriginatingDevice)
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
		bacnetNpduFromOriginatingDevice: util.CloneBytes(npdu),
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
		bacnetNpdu: util.CloneBytes(npdu),
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

	out, err := o.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode original-unicast-npdu: %w", err)
	}

	if copy(out[BVLCHeaderLen:], o.bacnetNpdu) != len(o.bacnetNpdu) {
		return nil, fmt.Errorf("encode original-unicast-npdu")
	}

	return out, nil
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

	res.bacnetNpdu = util.CloneBytes(data[BVLCHeaderLen:])
	*o = res
	return nil
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (o *OriginalUnicastNpdu) NPDUBytes() []byte {
	return util.CloneBytes(o.bacnetNpdu)
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
		bacnetNpdu: util.CloneBytes(npdu),
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

	res.bacnetNpdu = util.CloneBytes(data[BVLCHeaderLen:])
	*o = res
	return nil
}

// NPDUBytes returns a defensive copy of the enclosed NPDU payload.
func (o *OriginalBroadcastNpdu) NPDUBytes() []byte {
	return util.CloneBytes(o.bacnetNpdu)
}

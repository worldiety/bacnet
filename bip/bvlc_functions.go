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

type tableEntry struct {
	// the IP address of the gateway if NAT is active, of the target otherwise
	address netip.AddrPort
	// the subnet mask if NAT is active, 255.255.255.255 otherwise
	broadcastDistributionMask net.IPMask
}

type tableEntryKinds interface {
	BdtEntry | FdtEntry
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

func NewBdtEntry(address netip.AddrPort, broadcastDistributionMask net.IPMask) *BdtEntry {
	return &BdtEntry{
		address:                   address,
		broadcastDistributionMask: broadcastDistributionMask,
	}
}

type entryList[T tableEntryKinds] []T

func (l *entryList[T]) Decode(data []byte) error {
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

	entriesValid := true

	for _, e := range w.bdtEntries {
		entriesValid = entriesValid && e.Valid()
	}

	return w.header.Valid() && entriesValid
}

func (w *WriteBroadcastDistributionTable) Encode() ([]byte, error) {
	if w == nil {
		return nil, fmt.Errorf("cannot encode nil bvlc-write-broadcast-distribution-table")
	}

	out, err := w.header.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode bvlc-write-broadcast-distribution-table: %w", err)
	}

	for i, entry := range w.bdtEntries {
		entryBytes, err := entry.Encode()
		if err != nil {
			return nil, fmt.Errorf("encode bvlc-write-broadcast-distribution-table-entry %d: %w", i, err)
		}

		out = append(out, entryBytes...)
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

type ReadForeignDeviceTableAck struct {
	header  BVLCHeader
	entries BdtEntryList
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
		entries: make(BdtEntryList, 0),
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

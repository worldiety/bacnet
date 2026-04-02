package bip

import (
	"fmt"
	"net"
	"net/netip"

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

type BdtEntry struct {
	// the IP address of the gateway if NAT is active, of the target otherwise
	address netip.AddrPort
	// the subnet mask if NAT is active, 255.255.255.255 otherwise
	broadcastDistributionMask net.IPMask
}

func (b *BdtEntry) Valid() bool {
	if b == nil {
		return false
	}

	addressValid := b.address.Addr().Is4() && b.address.IsValid()

	maskValid := b.broadcastDistributionMask[3] >= b.broadcastDistributionMask[2] &&
		b.broadcastDistributionMask[2] >= b.broadcastDistributionMask[1] &&
		b.broadcastDistributionMask[1] >= b.broadcastDistributionMask[0]

	return addressValid && maskValid
}

const (
	BdtEntryDataLen = 10
)

func (b *BdtEntry) Encode() ([]byte, error) {
	out := make([]byte, BdtEntryDataLen)

	copy(out[0:4], b.address.Addr().AsSlice())
	out[5] = uint8(b.address.Port() >> 8)
	out[6] = uint8(b.address.Port() & 0xFF)

	copy(out[7:9], b.broadcastDistributionMask)

	return out, nil
}

func (b *BdtEntry) Decode(data []byte) error {
	if len(data) != BdtEntryDataLen {
		return fmt.Errorf("invalid length for bdt entry: %d", len(data))
	}

	ip, ok := netip.AddrFromSlice(data[0:4])
	if !ok {
		return fmt.Errorf("invalid ip in bdt entry")
	}

	port := uint16(data[4])<<8 | uint16(data[5])

	address := netip.AddrPortFrom(ip, port)

	if !address.IsValid() {
		return fmt.Errorf("invalid ip in bdt entry")
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

func NewBdtEntry(address netip.AddrPort, broadcastDistributionMask net.IPMask) *BdtEntry {
	return &BdtEntry{
		address:                   address,
		broadcastDistributionMask: broadcastDistributionMask,
	}
}

type WriteBroadCastDistributionTable struct {
	header     BVLCHeader
	bdtEntries []BdtEntry
}

func (w *WriteBroadCastDistributionTable) BVLCFunctionType() BVLCFunctionType {
	return FunctionWriteBroadcastDistributionTable
}

func (w *WriteBroadCastDistributionTable) Valid() bool {
	if w == nil {
		return false
	}

	entriesValid := true

	for _, e := range w.bdtEntries {
		entriesValid = entriesValid && e.Valid()
	}

	return w.header.Valid() && entriesValid
}

func (w *WriteBroadCastDistributionTable) Encode() ([]byte, error) {
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

func (w *WriteBroadCastDistributionTable) Decode(data []byte) error {
	if len(data) < BVLCHeaderLen+BdtEntryDataLen { // cannot contain less than one entry
		return fmt.Errorf("invalid length for bvlc-write-broadcast-distribution-table")
	}

	var header BVLCHeader
	headerBytes := data[:BVLCHeaderLen]
	err := header.Decode(headerBytes)
	if err != nil {
		return fmt.Errorf("decode bvlc-write-broadcast-distribution-table header: %w", err)
	}

	entries := make([]BdtEntry, 0)

	entriesBytes := data[BVLCHeaderLen:]

	for i := 0; i < len(entriesBytes); i += BdtEntryDataLen {
		entryBytes := entriesBytes[i : i+BdtEntryDataLen]
		var entry BdtEntry
		err = entry.Decode(entryBytes)
		if err != nil {
			return fmt.Errorf("decode bvlc-write-broadcast-distribution-table entry %d: %w", i, err)
		}

		entries = append(entries, entry)
	}

	res := WriteBroadCastDistributionTable{
		header:     header,
		bdtEntries: entries,
	}
	if !res.Valid() {
		return fmt.Errorf("decoded invalid bvlc-write-broadcast-distribution-table")
	}

	*w = res

	return nil
}

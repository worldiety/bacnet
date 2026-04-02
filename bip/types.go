package bip

import (
	"fmt"
	"math"

	"go.wdy.de/bacnet"
)

const (
	// BVLCHeaderLen is the size of the Annex J BVLC header in bytes.
	BVLCHeaderLen = 4
)

type BVLCHeader struct {
	BVLCType
	BVLCFunctionType
	BVLCLength
}

func (h *BVLCHeader) Valid() bool {
	return h.BVLCType.Valid() && h.BVLCFunctionType.Valid()
}

// Encode encodes the header into the first four bytes a []byte of size header.BVLCLength and returns it, the rest of the slice is
// available for BVLC function encoding.
func (h *BVLCHeader) Encode() ([]byte, error) {
	if h == nil {
		return nil, fmt.Errorf("nil bvlc-header")
	}
	if !h.BVLCType.Valid() {
		return nil, ErrInvalidBVLCType
	}
	if !h.BVLCFunctionType.Valid() {
		return nil, ErrInvalidFunction
	}
	if h.BVLCLength <= BVLCHeaderLen {
		return nil, ErrInvalidLength
	}

	out := make([]byte, h.BVLCLength)

	out[0] = byte(h.BVLCType)
	out[1] = byte(h.BVLCFunctionType)
	out[2] = byte(h.BVLCLength >> 8)
	out[3] = byte(h.BVLCLength)

	return out, nil
}

func (h *BVLCHeader) Decode(data []byte) error {
	if len(data) != BVLCHeaderLen {
		return fmt.Errorf("buffer too small to contain BVLC header")
	}

	bvclType := BVLCType(data[0])
	bvclFunctionType := BVLCFunctionType(data[1])
	bvclLength := BVLCLength(uint16(data[2])<<8 | uint16(data[3]))

	if !bvclType.Valid() {
		return fmt.Errorf("header BVLCType not valid")
	}

	if !bvclFunctionType.Valid() {
		return fmt.Errorf("header BVLCFunction not valid")
	}

	*h = BVLCHeader{
		BVLCType:         bvclType,
		BVLCFunctionType: bvclFunctionType,
		BVLCLength:       bvclLength,
	}

	return nil
}

type BVLCType byte

// Constants for possible BVLCType values, constants may be added when needed, values must be BVLCType values defined in the BACnet standard
const (
	BVLCTypeBACnetIP  BVLCType = 0x81
	BVLCTypeBACnetIP6 BVLCType = 0x82
)

func (t BVLCType) Valid() bool {
	// BVLCType must have one of the constant values defined in the const block above, other values are invalid,
	return t == BVLCTypeBACnetIP || t == BVLCTypeBACnetIP6
}

// IsIp4 returns true if BVLC type is 0x81 (=IP4), implies valid
func (t BVLCType) IsIp4() bool {
	return t == BVLCTypeBACnetIP
}

// IsIp6 returns true if BVLC type is 0x82 (=IP6), implies valid
func (t BVLCType) IsIp6() bool {
	return t == BVLCTypeBACnetIP6
}

func (t BVLCType) String() string {
	switch t {
	case BVLCTypeBACnetIP:
		return "bacnet-ip"
	case BVLCTypeBACnetIP6:
		return "bacnet-ip6"
	default:
		return fmt.Sprintf("bvlc-type(%d)", t)
	}
}

type BVLCLength uint16

func NewBVLCLength(length int) (BVLCLength, error) {
	if length < 0 || length > math.MaxUint16 {
		return 0, bacnet.NewValidationError("BVLCLength", length, ErrInvalidLength)
	}

	return BVLCLength(length), nil
}

// BVLCFunctionType identifies a BVLC function code.
type BVLCFunctionType uint8

// Constants for BVLCFunctionType codes as defined in Annex J of the BACnet standard (ANSI/ASHRAE 135-2024)
const (
	FunctionResult                            BVLCFunctionType = 0x00
	FunctionWriteBroadcastDistributionTable   BVLCFunctionType = 0x01
	FunctionReadBroadcastDistributionTable    BVLCFunctionType = 0x02
	FunctionReadBroadcastDistributionTableAck BVLCFunctionType = 0x03
	FunctionForwardedNPDU                     BVLCFunctionType = 0x04
	FunctionRegisterForeignDevice             BVLCFunctionType = 0x05
	FunctionReadForeignDeviceTable            BVLCFunctionType = 0x06
	FunctionReadForeignDeviceTableAck         BVLCFunctionType = 0x07
	FunctionDeleteForeignDeviceTableEntry     BVLCFunctionType = 0x08
	FunctionDistributeBroadcastToNetwork      BVLCFunctionType = 0x09
	FunctionOriginalUnicastNPDU               BVLCFunctionType = 0x0A
	FunctionOriginalBroadcastNPDU             BVLCFunctionType = 0x0B
)

func (f BVLCFunctionType) Valid() bool {
	return f <= FunctionOriginalBroadcastNPDU
}

func (f BVLCFunctionType) String() string {
	switch f {
	case FunctionResult:
		return "result"
	case FunctionWriteBroadcastDistributionTable:
		return "write-broadcast-distribution-table"
	case FunctionReadBroadcastDistributionTable:
		return "read-broadcast-distribution-table"
	case FunctionReadBroadcastDistributionTableAck:
		return "read-broadcast-distribution-table-ack"
	case FunctionForwardedNPDU:
		return "forwarded-npdu"
	case FunctionRegisterForeignDevice:
		return "register-foreign-device"
	case FunctionReadForeignDeviceTable:
		return "read-foreign-device-table"
	case FunctionReadForeignDeviceTableAck:
		return "read-foreign-device-table-ack"
	case FunctionDeleteForeignDeviceTableEntry:
		return "delete-foreign-device-table-entry"
	case FunctionDistributeBroadcastToNetwork:
		return "distribute-broadcast-to-network"
	case FunctionOriginalUnicastNPDU:
		return "original-unicast-npdu"
	case FunctionOriginalBroadcastNPDU:
		return "original-broadcast-npdu"
	default:
		return fmt.Sprintf("bvlc-function(%d)", f)
	}
}

// BVLCFunction is an interface implemented by concrete BTLC functions TODO do that ;)
type BVLCFunction interface {
	BVLCFunctionType() BVLCFunctionType
	Valid() bool
	Encode() ([]byte, error)
	Decode([]byte) error
}

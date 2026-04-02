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
	BVLCFunction
	BVLCLength
}

func (h *BVLCHeader) Valid() bool {
	return h.BVLCType.Valid() && h.BVLCFunction.Valid()
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

// BVLCFunction identifies a BVLC function code.
type BVLCFunction uint8

// Constants for BVLCFunction codes as defined in Annex J of the BACnet standard (ANSI/ASHRAE 135-2024)
const (
	FunctionResult                            BVLCFunction = 0x00
	FunctionWriteBroadcastDistributionTable   BVLCFunction = 0x01
	FunctionReadBroadcastDistributionTable    BVLCFunction = 0x02
	FunctionReadBroadcastDistributionTableAck BVLCFunction = 0x03
	FunctionForwardedNPDU                     BVLCFunction = 0x04
	FunctionRegisterForeignDevice             BVLCFunction = 0x05
	FunctionReadForeignDeviceTable            BVLCFunction = 0x06
	FunctionReadForeignDeviceTableAck         BVLCFunction = 0x07
	FunctionDeleteForeignDeviceTableEntry     BVLCFunction = 0x08
	FunctionDistributeBroadcastToNetwork      BVLCFunction = 0x09
	FunctionOriginalUnicastNPDU               BVLCFunction = 0x0A
	FunctionOriginalBroadcastNPDU             BVLCFunction = 0x0B
)

func (f BVLCFunction) Valid() bool {
	return f <= FunctionOriginalBroadcastNPDU
}

func (f BVLCFunction) String() string {
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

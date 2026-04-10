package bacnet

import (
	"fmt"
)

const (
	// ProtocolVersion is the BACnet protocol version defined by the standard.
	ProtocolVersion byte = 0x01

	// MaxInstanceNumber is the maximum BACnet object instance number.
	MaxInstanceNumber uint32 = (1 << 22) - 1

	// MaxObjectType is the maximum standard object type that fits in an object identifier.
	MaxObjectType ObjectType = (1 << 10) - 1
)

const (
	// LocalNetwork represents the local BACnet network number.
	LocalNetwork NetworkNumber = 0

	// GlobalBroadcastNetwork represents the BACnet global broadcast network.
	GlobalBroadcastNetwork NetworkNumber = 0xFFFF
)

const (
	// IpDefaultUdpPort is the default UDP port for BACnet/IP (0xBAC0 / 47808).
	IpDefaultUdpPort uint16 = 0xbac0
)

// NetworkPriority is the 2-bit message-priority field of the NPCI,
// per clause 6.2.2, Table 6-1 of ANSI/ASHRAE 135-2024.
type NetworkPriority uint8

const (
	// NetworkPriorityNormal is normal-priority message delivery (lowest).
	NetworkPriorityNormal NetworkPriority = 0b00
	// NetworkPriorityUrgent is urgent-priority message delivery.
	NetworkPriorityUrgent NetworkPriority = 0b01
	// NetworkPriorityCriticalEquipment is critical-equipment-priority message delivery.
	NetworkPriorityCriticalEquipment NetworkPriority = 0b10
	// NetworkPriorityLifeSafety is life-safety-priority message delivery (highest).
	NetworkPriorityLifeSafety NetworkPriority = 0b11
)

// Valid reports whether the value is one of the four defined NetworkPriority codes.
func (n NetworkPriority) Valid() bool {
	return n <= NetworkPriorityLifeSafety
}

func (n NetworkPriority) String() string {
	switch n {
	case NetworkPriorityNormal:
		return "normal"
	case NetworkPriorityUrgent:
		return "urgent"
	case NetworkPriorityCriticalEquipment:
		return "critical-equipment"
	case NetworkPriorityLifeSafety:
		return "life-safety"
	default:
		return fmt.Sprintf("network-priority(%d)", n)
	}
}

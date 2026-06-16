package netprim

import (
	"fmt"
)

// ProtocolVersion is the BACnet protocol version. It is always 0x01.
const ProtocolVersion byte = 0x01

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

// NetworkNumber identifies a BACnet network.
type NetworkNumber uint16

// IsLocal reports whether the network is the local BACnet network.
func (n NetworkNumber) IsLocal() bool {
	return n == LocalNetwork
}

// IsGlobalBroadcast reports whether the network is the global broadcast network.
func (n NetworkNumber) IsGlobalBroadcast() bool {
	return n == GlobalBroadcastNetwork
}

package bacnet

const (
	// ProtocolVersion is the BACnet protocol version defined by the standard.
	ProtocolVersion byte = 0x01

	// DefaultPort is the default UDP port for BACnet/IP (0xBAC0 / 47808).
	DefaultPort uint16 = 0xBAC0

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

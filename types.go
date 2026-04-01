package bacnet

import "fmt"

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

// DeviceInstance identifies a BACnet device object instance.
type DeviceInstance uint32

// NewDeviceInstance constructs a validated BACnet device instance.
func NewDeviceInstance(instance uint32) (DeviceInstance, error) {
	if instance > MaxInstanceNumber {
		return 0, &ValidationError{Field: "device instance", Value: instance, Err: ErrInvalidDeviceInstance}
	}
	return DeviceInstance(instance), nil
}

// Valid reports whether the device instance is within the BACnet range.
func (d DeviceInstance) Valid() bool {
	return uint32(d) <= MaxInstanceNumber
}

// ObjectType identifies a BACnet object type.
type ObjectType uint16

const (
	ObjectTypeAnalogInput       ObjectType = 0
	ObjectTypeAnalogOutput      ObjectType = 1
	ObjectTypeAnalogValue       ObjectType = 2
	ObjectTypeBinaryInput       ObjectType = 3
	ObjectTypeBinaryOutput      ObjectType = 4
	ObjectTypeBinaryValue       ObjectType = 5
	ObjectTypeDevice            ObjectType = 8
	ObjectTypeFile              ObjectType = 10
	ObjectTypeLoop              ObjectType = 12
	ObjectTypeMultiStateInput   ObjectType = 13
	ObjectTypeMultiStateOutput  ObjectType = 14
	ObjectTypeNotificationClass ObjectType = 15
)

// Valid reports whether the object type fits into a BACnet object identifier.
func (o ObjectType) Valid() bool {
	return o <= MaxObjectType
}

func (o ObjectType) String() string {
	switch o {
	case ObjectTypeAnalogInput:
		return "analog-input"
	case ObjectTypeAnalogOutput:
		return "analog-output"
	case ObjectTypeAnalogValue:
		return "analog-value"
	case ObjectTypeBinaryInput:
		return "binary-input"
	case ObjectTypeBinaryOutput:
		return "binary-output"
	case ObjectTypeBinaryValue:
		return "binary-value"
	case ObjectTypeDevice:
		return "device"
	case ObjectTypeFile:
		return "file"
	case ObjectTypeLoop:
		return "loop"
	case ObjectTypeMultiStateInput:
		return "multi-state-input"
	case ObjectTypeMultiStateOutput:
		return "multi-state-output"
	case ObjectTypeNotificationClass:
		return "notification-class"
	default:
		return fmt.Sprintf("object-type(%d)", o)
	}
}

// ObjectIdentifier is the 32-bit BACnet object identifier.
type ObjectIdentifier uint32

// NewObjectIdentifier constructs a validated BACnet object identifier.
func NewObjectIdentifier(objectType ObjectType, instance uint32) (ObjectIdentifier, error) {
	if !objectType.Valid() {
		return 0, &ValidationError{Field: "object type", Value: objectType, Err: ErrInvalidObjectType}
	}
	if instance > MaxInstanceNumber {
		return 0, &ValidationError{Field: "object instance", Value: instance, Err: ErrInvalidObjectInstance}
	}

	return ObjectIdentifier((uint32(objectType) << 22) | instance), nil
}

// ObjectType returns the BACnet object type portion of the identifier.
func (id ObjectIdentifier) ObjectType() ObjectType {
	return ObjectType(uint32(id) >> 22)
}

// Instance returns the BACnet object instance portion of the identifier.
func (id ObjectIdentifier) Instance() uint32 {
	return uint32(id) & MaxInstanceNumber
}

func (id ObjectIdentifier) String() string {
	return fmt.Sprintf("%s,%d", id.ObjectType(), id.Instance())
}

// PropertyIdentifier identifies a BACnet property.
type PropertyIdentifier uint32

const (
	PropertyIdentifierAckedTransitions           PropertyIdentifier = 0
	PropertyIdentifierObjectIdentifier           PropertyIdentifier = 75
	PropertyIdentifierObjectName                 PropertyIdentifier = 77
	PropertyIdentifierObjectType                 PropertyIdentifier = 79
	PropertyIdentifierPresentValue               PropertyIdentifier = 85
	PropertyIdentifierDescription                PropertyIdentifier = 28
	PropertyIdentifierStatusFlags                PropertyIdentifier = 111
	PropertyIdentifierUnits                      PropertyIdentifier = 117
	PropertyIdentifierVendorName                 PropertyIdentifier = 121
	PropertyIdentifierProtocolVersion            PropertyIdentifier = 98
	PropertyIdentifierProtocolRevision           PropertyIdentifier = 96
	PropertyIdentifierApplicationSoftwareVersion PropertyIdentifier = 12
)

func (p PropertyIdentifier) String() string {
	switch p {
	case PropertyIdentifierAckedTransitions:
		return "acked-transitions"
	case PropertyIdentifierApplicationSoftwareVersion:
		return "application-software-version"
	case PropertyIdentifierDescription:
		return "description"
	case PropertyIdentifierObjectIdentifier:
		return "object-identifier"
	case PropertyIdentifierObjectName:
		return "object-name"
	case PropertyIdentifierObjectType:
		return "object-type"
	case PropertyIdentifierPresentValue:
		return "present-value"
	case PropertyIdentifierProtocolRevision:
		return "protocol-revision"
	case PropertyIdentifierProtocolVersion:
		return "protocol-version"
	case PropertyIdentifierStatusFlags:
		return "status-flags"
	case PropertyIdentifierUnits:
		return "units"
	case PropertyIdentifierVendorName:
		return "vendor-name"
	default:
		return fmt.Sprintf("property-identifier(%d)", p)
	}
}

package types

import (
	"fmt"

	"go.wdy.de/bacnet/common/errors"
	"go.wdy.de/bacnet/common/netprim"
)

// DeviceInstance identifies a BACnet device object instance.
type DeviceInstance uint32

// NewDeviceInstance constructs a validated BACnet device instance.
func NewDeviceInstance(instance uint32) (DeviceInstance, error) {
	if instance > netprim.MaxInstanceNumber {
		return 0, errors.NewValidationError("device instance", instance, errors.ErrInvalidDeviceInstance)
	}
	return DeviceInstance(instance), nil
}

// Valid reports whether the device instance is within the BACnet range.
func (d DeviceInstance) Valid() bool {
	return uint32(d) <= netprim.MaxInstanceNumber
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

	// ObjectTypeMax is the maximum valid value for an ObjectType.
	ObjectTypeMax ObjectType = (1 << 10) - 1
)

// Valid reports whether the object type fits into a BACnet object identifier.
func (o ObjectType) Valid() bool {
	return o <= ObjectTypeMax
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
		return 0, errors.NewValidationError("object type", objectType, errors.ErrInvalidObjectType)
	}

	if instance > netprim.MaxInstanceNumber {
		return 0, errors.NewValidationError("object instance", instance, errors.ErrInvalidObjectInstance)
	}

	return ObjectIdentifier((uint32(objectType) << 22) | instance), nil
}

// ObjectType returns the BACnet object type portion of the identifier.
func (id ObjectIdentifier) ObjectType() ObjectType {
	return ObjectType(id >> 22)
}

// Instance returns the BACnet object instance portion of the identifier.
func (id ObjectIdentifier) Instance() uint32 {
	return uint32(id) & netprim.MaxInstanceNumber
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

type RejectReason uint8

const (
	RejectReasonOther                    RejectReason = 0
	RejectReasonBufferOverflow           RejectReason = 1
	RejectReasonInconsistentParameters   RejectReason = 2
	RejectReasonInvalidParameterDataType RejectReason = 3
	RejectReasonInvalidTag               RejectReason = 4
	RejectReasonMissingRequiredParameter RejectReason = 5
	RejectReasonParameterOutOfRange      RejectReason = 6
	RejectReasonTooManyArguments         RejectReason = 7
	RejectReasonUndefinedArguments       RejectReason = 8
	RejectReasonUnrecognizedService      RejectReason = 9
	RejectReasonInvalidDataEncoding      RejectReason = 10
)

// ValidStandard reports whether r is a defined standard BACnet reject reason.
func (r RejectReason) ValidStandard() bool {
	switch r {
	case RejectReasonOther,
		RejectReasonBufferOverflow,
		RejectReasonInconsistentParameters,
		RejectReasonInvalidParameterDataType,
		RejectReasonInvalidTag,
		RejectReasonMissingRequiredParameter,
		RejectReasonParameterOutOfRange,
		RejectReasonTooManyArguments,
		RejectReasonUndefinedArguments,
		RejectReasonUnrecognizedService,
		RejectReasonInvalidDataEncoding:
		return true
	default:
		return false
	}
}

func (r RejectReason) String() string {
	switch r {
	case RejectReasonOther:
		return "other"
	case RejectReasonBufferOverflow:
		return "buffer-overflow"
	case RejectReasonInconsistentParameters:
		return "inconsistent-parameters"
	case RejectReasonInvalidParameterDataType:
		return "invalid-parameter-data-type"
	case RejectReasonInvalidTag:
		return "invalid-tag"
	case RejectReasonMissingRequiredParameter:
		return "missing-required-parameter"
	case RejectReasonParameterOutOfRange:
		return "parameter-out-of-range"
	case RejectReasonTooManyArguments:
		return "too-many-arguments"
	case RejectReasonUndefinedArguments:
		return "undefined-arguments"
	case RejectReasonUnrecognizedService:
		return "unrecognized-service"
	case RejectReasonInvalidDataEncoding:
		return "invalid-data-encoding"
	default:
		return fmt.Sprintf("reject-reason(%d)", r)
	}
}

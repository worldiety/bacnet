package apdu

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidASEConfig = errors.New("invalid ASE config")
	ErrNilTransport     = errors.New("NPDU transport is required")
	ErrNilASE           = errors.New("ASE is required")

	ErrInvalidPDUType         = errors.New("invalid PDU type")
	ErrInvalidServiceChoice   = errors.New("invalid service choice")
	ErrInvalidStateTransition = errors.New("invalid application protocol state transition")

	ErrTransactionNotReady = errors.New("transaction not ready")

	ErrHandlerAlreadyRegistered = errors.New("handler already registered")
	ErrHandlerNotFound          = errors.New("handler not found")

	ErrNoInvokeIDAvailable = errors.New("no invoke ID available")
	ErrInvokeIDInUse       = errors.New("invoke ID in use")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrAPDUTimeout         = errors.New("APDU timeout")
	ErrASEClosed           = errors.New("ASE is closed")

	ErrDecodeFailure            = errors.New("decode failure")
	ErrEncodeFailure            = errors.New("encode failure")
	ErrTransportFailure         = errors.New("transport failure")
	ErrUnexpectedPDU            = errors.New("unexpected PDU received")
	ErrSecurityError            = errors.New("security error received")
	ErrSegmentationNotSupported = errors.New("segmentation required but not supported")

	ErrRemoteError  = errors.New("remote error APDU")
	ErrRemoteReject = errors.New("remote reject APDU")
	ErrRemoteAbort  = errors.New("remote abort APDU")
)

/*
	The development of this library was supported by a colony of common gulls (Larus canus) breeding on the roof outside
	the office windows, and voicing vocal concerns about human presence near their nests. Once the chicks were bigger
	you could even see them against the gravel roof.
*/

// ErrorClass represents a BACnet error class as defined in clause 18 of the standard.
type ErrorClass uint32

const (
	ErrorClassDevice        = ErrorClass(0) // device
	ErrorClassObject        = ErrorClass(1) // object
	ErrorClassProperty      = ErrorClass(2) // property
	ErrorClassResources     = ErrorClass(3) // resources
	ErrorClassSecurity      = ErrorClass(4) // security
	ErrorClassServices      = ErrorClass(5) // services
	ErrorClassVT            = ErrorClass(6) // vt
	ErrorClassCommunication = ErrorClass(7) // communication

	// ErrorClassUnknown is returned when the error-class field of an Error-PDU
	// could not be decoded (malformed payload).
	ErrorClassUnknown = ErrorClass(math.MaxUint32)
)

func (e ErrorClass) String() string {
	switch e {
	case ErrorClassDevice:
		return "device"
	case ErrorClassObject:
		return "object"
	case ErrorClassProperty:
		return "property"
	case ErrorClassResources:
		return "resources"
	case ErrorClassSecurity:
		return "security"
	case ErrorClassServices:
		return "services"
	case ErrorClassVT:
		return "vt"
	case ErrorClassCommunication:
		return "communication"
	case ErrorClassUnknown:
		return "error-class(unknown)"
	default:
		return fmt.Sprintf("error-class(%d)", uint32(e))
	}
}

// Valid reports whether e is a standard BACnet error class value (clause 18.24.1).
func (e ErrorClass) Valid() bool {
	return e <= ErrorClassCommunication
}

// ErrorCode represents a BACnet error code as defined in clause 18 of the standard.
// Each error code belongs to a primary error class; use Class to retrieve it.
type ErrorCode uint32

const (
	ErrorCodeOther = ErrorCode(0)
	// device class
	ErrorCodeDeviceConfigurationInProgress   = ErrorCode(2)
	ErrorCodeDeviceBusy                      = ErrorCode(3)
	ErrorCodeDeviceOperationalProblem        = ErrorCode(25)
	ErrorCodeDeviceInconsistentConfiguration = ErrorCode(129)
	ErrorCodeDeviceInternalError             = ErrorCode(131)
	ErrorCodeDeviceNotConfigured             = ErrorCode(132)

	// object class
	ErrorCodeObjectDynamicCreationNotSupported = ErrorCode(4)
	ErrorCodeObjectNoObjectsOfSpecifiedType    = ErrorCode(17)
	ErrorCodeObjectDeletionNotPermitted        = ErrorCode(23)
	ErrorCodeObjectIdentifierAlreadyExists     = ErrorCode(24)
	ErrorCodeObjectUnknownObject               = ErrorCode(31)
	ErrorCodeObjectUnsupportedObjectType       = ErrorCode(36)
	ErrorCodeObjectNoAlarmConfigured           = ErrorCode(74)
	ErrorCodeObjectLogBufferFull               = ErrorCode(75)
	ErrorCodeObjectBusy                        = ErrorCode(82)
	ErrorCodeObjectFileFull                    = ErrorCode(128)
	ErrorCodeObjectInvalidOperationInThisState = ErrorCode(139)

	// property class
	ErrorCodePropertyInconsistentSelectionCriterion    = ErrorCode(8)
	ErrorCodePropertyInvalidDataType                   = ErrorCode(9)
	ErrorCodePropertyReadAccessDenied                  = ErrorCode(27)
	ErrorCodePropertyUnknownProperty                   = ErrorCode(32)
	ErrorCodePropertyValueOutOfRange                   = ErrorCode(37)
	ErrorCodePropertyWriteAccessDenied                 = ErrorCode(40)
	ErrorCodePropertyCharacterSetNotSupported          = ErrorCode(41)
	ErrorCodePropertyInvalidArrayIndex                 = ErrorCode(42)
	ErrorCodePropertyNotCovProperty                    = ErrorCode(44)
	ErrorCodePropertyOptionalFunctionalityNotSupported = ErrorCode(45)
	ErrorCodePropertyDatatypeNotSupported              = ErrorCode(47)
	ErrorCodePropertyDuplicateName                     = ErrorCode(48)
	ErrorCodePropertyDuplicateObjectId                 = ErrorCode(49)
	ErrorCodePropertyIsNotAnArray                      = ErrorCode(50)
	ErrorCodePropertyValueNotInitialized               = ErrorCode(72)
	ErrorCodePropertyLoggedValuePurged                 = ErrorCode(76)
	ErrorCodePropertyNoPropertySpecified               = ErrorCode(77)
	ErrorCodePropertyNotConfiguredForTriggeredLogging  = ErrorCode(78)
	ErrorCodePropertyUnknownFileSize                   = ErrorCode(122)
	ErrorCodePropertyValueTooLong                      = ErrorCode(134)
	ErrorCodePropertyDuplicateEntry                    = ErrorCode(137)
	ErrorCodePropertyInvalidValueInThisState           = ErrorCode(138)
	ErrorCodePropertyListItemNotNumbered               = ErrorCode(140)
	ErrorCodePropertyListItemNotTimestamped            = ErrorCode(141)
	ErrorCodePropertyInvalidDataEncoding               = ErrorCode(142)

	// resources class
	ErrorCodeResourcesNoSpaceForObject        = ErrorCode(18)
	ErrorCodeResourcesNoSpaceToAddListElement = ErrorCode(19)
	ErrorCodeResourcesNoSpaceToWriteProperty  = ErrorCode(20)
	ErrorCodeResourcesOutOfMemory             = ErrorCode(133)

	// security class
	ErrorCodeSecurityPasswordFailure             = ErrorCode(26)
	ErrorCodeSecuritySuccess                     = ErrorCode(84)
	ErrorCodeSecurityAccessDenied                = ErrorCode(85)
	ErrorCodeSecurityBadDestinationAddress       = ErrorCode(86)
	ErrorCodeSecurityBadDestinationDeviceId      = ErrorCode(87)
	ErrorCodeSecurityBadSignature                = ErrorCode(88)
	ErrorCodeSecurityBadSourceAddress            = ErrorCode(89)
	ErrorCodeSecurityBadTimestamp                = ErrorCode(90) // removed in revision 22; kept for decode compatibility
	ErrorCodeSecurityCannotUseKey                = ErrorCode(91) // removed in revision 22
	ErrorCodeSecurityCannotVerifyMessageId       = ErrorCode(92) // removed in revision 22
	ErrorCodeSecurityCorrectKeyRevision          = ErrorCode(93) // removed in revision 22
	ErrorCodeSecurityDestinationDeviceIdRequired = ErrorCode(94) // removed in revision 22
	ErrorCodeSecurityDuplicateMessage            = ErrorCode(95)
	ErrorCodeSecurityEncryptionNotConfigured     = ErrorCode(96)
	ErrorCodeSecurityEncryptionRequired          = ErrorCode(97)
	ErrorCodeSecurityIncorrectKey                = ErrorCode(98)  // removed in revision 22
	ErrorCodeSecurityInvalidKeyData              = ErrorCode(99)  // removed in revision 22
	ErrorCodeSecurityKeyUpdateInProgress         = ErrorCode(100) // removed in revision 22
	ErrorCodeSecurityMalformedMessage            = ErrorCode(101)
	ErrorCodeSecurityNotKeyServer                = ErrorCode(102) // removed in revision 22
	ErrorCodeSecurityNotConfigured               = ErrorCode(103)
	ErrorCodeSecuritySourceSecurityRequired      = ErrorCode(104)
	ErrorCodeSecurityTooManyKeys                 = ErrorCode(105) // removed in revision 22
	ErrorCodeSecurityUnknownAuthenticationType   = ErrorCode(106)
	ErrorCodeSecurityUnknownKey                  = ErrorCode(107) // removed in revision 22
	ErrorCodeSecurityUnknownKeyRevision          = ErrorCode(108) // removed in revision 22
	ErrorCodeSecurityUnknownSourceMessage        = ErrorCode(109) // removed in revision 22
	ErrorCodeSecurityCertificateExpired          = ErrorCode(200)
	ErrorCodeSecurityCertificateInvalid          = ErrorCode(201)
	ErrorCodeSecurityCertificateMalformed        = ErrorCode(202)
	ErrorCodeSecurityCertificateRevoked          = ErrorCode(203)

	// services class
	ErrorCodeServicesFileAccessDenied         = ErrorCode(5)
	ErrorCodeServicesInconsistentParameters   = ErrorCode(7)
	ErrorCodeServicesInvalidFileAccessMethod  = ErrorCode(10)
	ErrorCodeServicesInvalidFileStartPosition = ErrorCode(11)
	ErrorCodeServicesInvalidParameterDataType = ErrorCode(13)
	ErrorCodeServicesInvalidTimestamp         = ErrorCode(14)
	ErrorCodeServicesMissingRequiredParameter = ErrorCode(16)
	ErrorCodeServicesPropertyIsNotAList       = ErrorCode(22)
	ErrorCodeServicesServiceRequestDenied     = ErrorCode(29)
	ErrorCodeServicesCovSubscriptionFailed    = ErrorCode(43)
	ErrorCodeServicesInvalidConfigurationData = ErrorCode(46)
	ErrorCodeServicesInvalidTag               = ErrorCode(57)
	ErrorCodeServicesInvalidEventState        = ErrorCode(73)
	ErrorCodeServicesUnknownSubscription      = ErrorCode(79)
	ErrorCodeServicesParameterOutOfRange      = ErrorCode(80)
	ErrorCodeServicesListElementNotFound      = ErrorCode(81)
	ErrorCodeServicesCommunicationDisabled    = ErrorCode(83)
	ErrorCodeServicesInconsistentObjectType   = ErrorCode(130)

	// vt class
	ErrorCodeVTNoVtSessionsAvailable       = ErrorCode(21)
	ErrorCodeVTUnknownVtClass              = ErrorCode(34)
	ErrorCodeVTUnknownVtSession            = ErrorCode(35)
	ErrorCodeVTVtSessionAlreadyClosed      = ErrorCode(38)
	ErrorCodeVTVtSessionTerminationFailure = ErrorCode(39)

	// communication class
	ErrorCodeCommunicationTimeout                            = ErrorCode(30)
	ErrorCodeCommunicationAbortBufferOverflow                = ErrorCode(51)
	ErrorCodeCommunicationAbortInvalidApduInThisState        = ErrorCode(52)
	ErrorCodeCommunicationAbortPreemptedByHigherPriorityTask = ErrorCode(53)
	ErrorCodeCommunicationAbortSegmentationNotSupported      = ErrorCode(54)
	ErrorCodeCommunicationAbortProprietary                   = ErrorCode(55)
	ErrorCodeCommunicationAbortOther                         = ErrorCode(56)
	ErrorCodeCommunicationNetworkDown                        = ErrorCode(58)
	ErrorCodeCommunicationRejectBufferOverflow               = ErrorCode(59)
	ErrorCodeCommunicationRejectInconsistentParameters       = ErrorCode(60)
	ErrorCodeCommunicationRejectInvalidParameterDataType     = ErrorCode(61)
	ErrorCodeCommunicationRejectInvalidTag                   = ErrorCode(62)
	ErrorCodeCommunicationRejectMissingRequiredParameter     = ErrorCode(63)
	ErrorCodeCommunicationRejectParameterOutOfRange          = ErrorCode(64)
	ErrorCodeCommunicationRejectTooManyArguments             = ErrorCode(65)
	ErrorCodeCommunicationRejectUndefinedEnumeration         = ErrorCode(66)
	ErrorCodeCommunicationRejectUnrecognizedService          = ErrorCode(67)
	ErrorCodeCommunicationRejectProprietary                  = ErrorCode(68)
	ErrorCodeCommunicationRejectOther                        = ErrorCode(69)
	ErrorCodeCommunicationUnknownDevice                      = ErrorCode(70)
	ErrorCodeCommunicationUnknownRoute                       = ErrorCode(71)
	ErrorCodeCommunicationNotRouterToDnet                    = ErrorCode(110)
	ErrorCodeCommunicationRouterBusy                         = ErrorCode(111)
	ErrorCodeCommunicationUnknownNetworkMessage              = ErrorCode(112)
	ErrorCodeCommunicationMessageTooLong                     = ErrorCode(113)
	ErrorCodeCommunicationSecurityError                      = ErrorCode(114)
	ErrorCodeCommunicationAddressingError                    = ErrorCode(115)
	ErrorCodeCommunicationWriteBdtFailed                     = ErrorCode(116)
	ErrorCodeCommunicationReadBdtFailed                      = ErrorCode(117)
	ErrorCodeCommunicationRegisterForeignDeviceFailed        = ErrorCode(118)
	ErrorCodeCommunicationReadFdtFailed                      = ErrorCode(119)
	ErrorCodeCommunicationDeleteFdtEntryFailed               = ErrorCode(120)
	ErrorCodeCommunicationDistributeBroadcastFailed          = ErrorCode(121)
	ErrorCodeCommunicationAbortApduTooLong                   = ErrorCode(123)
	ErrorCodeCommunicationAbortApplicationExceededReplyTime  = ErrorCode(124)
	ErrorCodeCommunicationAbortOutOfResources                = ErrorCode(125)
	ErrorCodeCommunicationAbortTsmTimeout                    = ErrorCode(126)
	ErrorCodeCommunicationAbortWindowSizeOutOfRange          = ErrorCode(127)
	ErrorCodeCommunicationAbortInsufficientSecurity          = ErrorCode(135)
	ErrorCodeCommunicationAbortSecurityError                 = ErrorCode(136)
	ErrorCodeCommunicationBvlcFunctionUnknown                = ErrorCode(143)
	ErrorCodeCommunicationBvlcProprietaryFunctionUnknown     = ErrorCode(144)
	ErrorCodeCommunicationHeaderEncodingError                = ErrorCode(145)
	ErrorCodeCommunicationHeaderNotUnderstood                = ErrorCode(146)
	ErrorCodeCommunicationMessageIncomplete                  = ErrorCode(147)
	ErrorCodeCommunicationNotABacnetScHub                    = ErrorCode(148)
	ErrorCodeCommunicationPayloadExpected                    = ErrorCode(149)
	ErrorCodeCommunicationUnexpectedData                     = ErrorCode(150)
	ErrorCodeCommunicationNodeDuplicateVmac                  = ErrorCode(151)
	ErrorCodeCommunicationHttpUnexpectedResponseCode         = ErrorCode(152)
	ErrorCodeCommunicationHttpNoUpgrade                      = ErrorCode(153)
	ErrorCodeCommunicationHttpResourceNotLocal               = ErrorCode(154)
	ErrorCodeCommunicationHttpProxyAuthenticationFailed      = ErrorCode(155)
	ErrorCodeCommunicationHttpResponseTimeout                = ErrorCode(156)
	ErrorCodeCommunicationHttpResponseSyntaxError            = ErrorCode(157)
	ErrorCodeCommunicationHttpResponseValueError             = ErrorCode(158)
	ErrorCodeCommunicationHttpResponseMissingHeader          = ErrorCode(159)
	ErrorCodeCommunicationHttpWebsocketHeaderError           = ErrorCode(160)
	ErrorCodeCommunicationHttpUpgradeRequired                = ErrorCode(161)
	ErrorCodeCommunicationHttpUpgradeError                   = ErrorCode(162)
	ErrorCodeCommunicationHttpTemporaryUnavailable           = ErrorCode(163)
	ErrorCodeCommunicationHttpNotAServer                     = ErrorCode(164)
	ErrorCodeCommunicationHttpError                          = ErrorCode(165)
	ErrorCodeCommunicationWebsocketSchemeNotSupported        = ErrorCode(166)
	ErrorCodeCommunicationWebsocketUnknownControlMessage     = ErrorCode(167)
	ErrorCodeCommunicationWebsocketCloseError                = ErrorCode(168)
	ErrorCodeCommunicationWebsocketClosedByPeer              = ErrorCode(169)
	ErrorCodeCommunicationWebsocketEndpointLeaves            = ErrorCode(170)
	ErrorCodeCommunicationWebsocketProtocolError             = ErrorCode(171)
	ErrorCodeCommunicationWebsocketDataNotAccepted           = ErrorCode(172)
	ErrorCodeCommunicationWebsocketClosedAbnormally          = ErrorCode(173)
	ErrorCodeCommunicationWebsocketDataInconsistent          = ErrorCode(174)
	ErrorCodeCommunicationWebsocketDataAgainstPolicy         = ErrorCode(175)
	ErrorCodeCommunicationWebsocketFrameTooLong              = ErrorCode(176)
	ErrorCodeCommunicationWebsocketExtensionMissing          = ErrorCode(177)
	ErrorCodeCommunicationWebsocketRequestUnavailable        = ErrorCode(178)
	ErrorCodeCommunicationWebsocketError                     = ErrorCode(179)
	ErrorCodeCommunicationTlsClientCertificateError          = ErrorCode(180)
	ErrorCodeCommunicationTlsServerCertificateError          = ErrorCode(181)
	ErrorCodeCommunicationTlsClientAuthenticationFailed      = ErrorCode(182)
	ErrorCodeCommunicationTlsServerAuthenticationFailed      = ErrorCode(183)
	ErrorCodeCommunicationTlsClientCertificateExpired        = ErrorCode(184)
	ErrorCodeCommunicationTlsServerCertificateExpired        = ErrorCode(185)
	ErrorCodeCommunicationTlsClientCertificateRevoked        = ErrorCode(186)
	ErrorCodeCommunicationTlsServerCertificateRevoked        = ErrorCode(187)
	ErrorCodeCommunicationTlsError                           = ErrorCode(188)
	ErrorCodeCommunicationDnsUnavailable                     = ErrorCode(189)
	ErrorCodeCommunicationDnsNameResolutionFailed            = ErrorCode(190)
	ErrorCodeCommunicationDnsResolverFailure                 = ErrorCode(191)
	ErrorCodeCommunicationDnsError                           = ErrorCode(192)
	ErrorCodeCommunicationTcpConnectTimeout                  = ErrorCode(193)
	ErrorCodeCommunicationTcpConnectionRefused               = ErrorCode(194)
	ErrorCodeCommunicationTcpClosedByLocal                   = ErrorCode(195)
	ErrorCodeCommunicationTcpClosedOther                     = ErrorCode(196)
	ErrorCodeCommunicationTcpError                           = ErrorCode(197)
	ErrorCodeCommunicationIpAddressNotReachable              = ErrorCode(198)
	ErrorCodeCommunicationIpError                            = ErrorCode(199)

	// ErrorCodeUnknown is returned when the error-code field of an Error-PDU
	// could not be decoded (malformed payload).
	ErrorCodeUnknown = ErrorCode(math.MaxUint32)
)

func (e ErrorCode) String() string {
	switch e {
	case ErrorCodeOther:
		return "other"
	// device
	case ErrorCodeDeviceConfigurationInProgress:
		return "configuration-in-progress"
	case ErrorCodeDeviceBusy:
		return "device-busy"
	case ErrorCodeDeviceOperationalProblem:
		return "operational-problem"
	case ErrorCodeDeviceInconsistentConfiguration:
		return "inconsistent-configuration"
	case ErrorCodeDeviceInternalError:
		return "internal-error"
	case ErrorCodeDeviceNotConfigured:
		return "not-configured"
	// object
	case ErrorCodeObjectDynamicCreationNotSupported:
		return "dynamic-creation-not-supported"
	case ErrorCodeObjectNoObjectsOfSpecifiedType:
		return "no-objects-of-specified-type"
	case ErrorCodeObjectDeletionNotPermitted:
		return "object-deletion-not-permitted"
	case ErrorCodeObjectIdentifierAlreadyExists:
		return "object-identifier-already-exists"
	case ErrorCodeObjectUnknownObject:
		return "unknown-object"
	case ErrorCodeObjectUnsupportedObjectType:
		return "unsupported-object-type"
	case ErrorCodeObjectNoAlarmConfigured:
		return "no-alarm-configured"
	case ErrorCodeObjectLogBufferFull:
		return "log-buffer-full"
	case ErrorCodeObjectBusy:
		return "busy"
	case ErrorCodeObjectFileFull:
		return "file-full"
	case ErrorCodeObjectInvalidOperationInThisState:
		return "invalid-operation-in-this-state"
	// property
	case ErrorCodePropertyInconsistentSelectionCriterion:
		return "inconsistent-selection-criterion"
	case ErrorCodePropertyInvalidDataType:
		return "invalid-data-type"
	case ErrorCodePropertyReadAccessDenied:
		return "read-access-denied"
	case ErrorCodePropertyUnknownProperty:
		return "unknown-property"
	case ErrorCodePropertyValueOutOfRange:
		return "value-out-of-range"
	case ErrorCodePropertyWriteAccessDenied:
		return "write-access-denied"
	case ErrorCodePropertyCharacterSetNotSupported:
		return "character-set-not-supported"
	case ErrorCodePropertyInvalidArrayIndex:
		return "invalid-array-index"
	case ErrorCodePropertyNotCovProperty:
		return "not-cov-property"
	case ErrorCodePropertyOptionalFunctionalityNotSupported:
		return "optional-functionality-not-supported"
	case ErrorCodePropertyDatatypeNotSupported:
		return "datatype-not-supported"
	case ErrorCodePropertyDuplicateName:
		return "duplicate-name"
	case ErrorCodePropertyDuplicateObjectId:
		return "duplicate-object-id"
	case ErrorCodePropertyIsNotAnArray:
		return "property-is-not-an-array"
	case ErrorCodePropertyValueNotInitialized:
		return "value-not-initialized"
	case ErrorCodePropertyLoggedValuePurged:
		return "logged-value-purged"
	case ErrorCodePropertyNoPropertySpecified:
		return "no-property-specified"
	case ErrorCodePropertyNotConfiguredForTriggeredLogging:
		return "not-configured-for-triggered-logging"
	case ErrorCodePropertyUnknownFileSize:
		return "unknown-file-size"
	case ErrorCodePropertyValueTooLong:
		return "value-too-long"
	case ErrorCodePropertyDuplicateEntry:
		return "duplicate-entry"
	case ErrorCodePropertyInvalidValueInThisState:
		return "invalid-value-in-this-state"
	case ErrorCodePropertyListItemNotNumbered:
		return "list-item-not-numbered"
	case ErrorCodePropertyListItemNotTimestamped:
		return "list-item-not-timestamped"
	case ErrorCodePropertyInvalidDataEncoding:
		return "invalid-data-encoding"
	// resources
	case ErrorCodeResourcesNoSpaceForObject:
		return "no-space-for-object"
	case ErrorCodeResourcesNoSpaceToAddListElement:
		return "no-space-to-add-list-element"
	case ErrorCodeResourcesNoSpaceToWriteProperty:
		return "no-space-to-write-property"
	case ErrorCodeResourcesOutOfMemory:
		return "out-of-memory"
	// security
	case ErrorCodeSecurityPasswordFailure:
		return "password-failure"
	case ErrorCodeSecuritySuccess:
		return "success"
	case ErrorCodeSecurityAccessDenied:
		return "access-denied"
	case ErrorCodeSecurityBadDestinationAddress:
		return "bad-destination-address"
	case ErrorCodeSecurityBadDestinationDeviceId:
		return "bad-destination-device-id"
	case ErrorCodeSecurityBadSignature:
		return "bad-signature"
	case ErrorCodeSecurityBadSourceAddress:
		return "bad-source-address"
	case ErrorCodeSecurityBadTimestamp:
		return "bad-timestamp"
	case ErrorCodeSecurityCannotUseKey:
		return "cannot-use-key"
	case ErrorCodeSecurityCannotVerifyMessageId:
		return "cannot-verify-message-id"
	case ErrorCodeSecurityCorrectKeyRevision:
		return "correct-key-revision"
	case ErrorCodeSecurityDestinationDeviceIdRequired:
		return "destination-device-id-required"
	case ErrorCodeSecurityDuplicateMessage:
		return "duplicate-message"
	case ErrorCodeSecurityEncryptionNotConfigured:
		return "encryption-not-configured"
	case ErrorCodeSecurityEncryptionRequired:
		return "encryption-required"
	case ErrorCodeSecurityIncorrectKey:
		return "incorrect-key"
	case ErrorCodeSecurityInvalidKeyData:
		return "invalid-key-data"
	case ErrorCodeSecurityKeyUpdateInProgress:
		return "key-update-in-progress"
	case ErrorCodeSecurityMalformedMessage:
		return "malformed-message"
	case ErrorCodeSecurityNotKeyServer:
		return "not-key-server"
	case ErrorCodeSecurityNotConfigured:
		return "security-not-configured"
	case ErrorCodeSecuritySourceSecurityRequired:
		return "source-security-required"
	case ErrorCodeSecurityTooManyKeys:
		return "too-many-keys"
	case ErrorCodeSecurityUnknownAuthenticationType:
		return "unknown-authentication-type"
	case ErrorCodeSecurityUnknownKey:
		return "unknown-key"
	case ErrorCodeSecurityUnknownKeyRevision:
		return "unknown-key-revision"
	case ErrorCodeSecurityUnknownSourceMessage:
		return "unknown-source-message"
	case ErrorCodeSecurityCertificateExpired:
		return "certificate-expired"
	case ErrorCodeSecurityCertificateInvalid:
		return "certificate-invalid"
	case ErrorCodeSecurityCertificateMalformed:
		return "certificate-malformed"
	case ErrorCodeSecurityCertificateRevoked:
		return "certificate-revoked"
	// services
	case ErrorCodeServicesFileAccessDenied:
		return "file-access-denied"
	case ErrorCodeServicesInconsistentParameters:
		return "inconsistent-parameters"
	case ErrorCodeServicesInvalidFileAccessMethod:
		return "invalid-file-access-method"
	case ErrorCodeServicesInvalidFileStartPosition:
		return "invalid-file-start-position"
	case ErrorCodeServicesInvalidParameterDataType:
		return "invalid-parameter-data-type"
	case ErrorCodeServicesInvalidTimestamp:
		return "invalid-time-stamp"
	case ErrorCodeServicesMissingRequiredParameter:
		return "missing-required-parameter"
	case ErrorCodeServicesPropertyIsNotAList:
		return "property-is-not-a-list"
	case ErrorCodeServicesServiceRequestDenied:
		return "service-request-denied"
	case ErrorCodeServicesCovSubscriptionFailed:
		return "cov-subscription-failed"
	case ErrorCodeServicesInvalidConfigurationData:
		return "invalid-configuration-data"
	case ErrorCodeServicesInvalidTag:
		return "invalid-tag"
	case ErrorCodeServicesInvalidEventState:
		return "invalid-event-state"
	case ErrorCodeServicesUnknownSubscription:
		return "unknown-subscription"
	case ErrorCodeServicesParameterOutOfRange:
		return "parameter-out-of-range"
	case ErrorCodeServicesListElementNotFound:
		return "list-element-not-found"
	case ErrorCodeServicesCommunicationDisabled:
		return "communication-disabled"
	case ErrorCodeServicesInconsistentObjectType:
		return "inconsistent-object-type"
	// vt
	case ErrorCodeVTNoVtSessionsAvailable:
		return "no-vt-sessions-available"
	case ErrorCodeVTUnknownVtClass:
		return "unknown-vt-class"
	case ErrorCodeVTUnknownVtSession:
		return "unknown-vt-session"
	case ErrorCodeVTVtSessionAlreadyClosed:
		return "vt-session-already-closed"
	case ErrorCodeVTVtSessionTerminationFailure:
		return "vt-session-termination-failure"
	// communication
	case ErrorCodeCommunicationTimeout:
		return "timeout"
	case ErrorCodeCommunicationAbortBufferOverflow:
		return "abort-buffer-overflow"
	case ErrorCodeCommunicationAbortInvalidApduInThisState:
		return "abort-invalid-apdu-in-this-state"
	case ErrorCodeCommunicationAbortPreemptedByHigherPriorityTask:
		return "abort-preempted-by-higher-priority-task"
	case ErrorCodeCommunicationAbortSegmentationNotSupported:
		return "abort-segmentation-not-supported"
	case ErrorCodeCommunicationAbortProprietary:
		return "abort-proprietary"
	case ErrorCodeCommunicationAbortOther:
		return "abort-other"
	case ErrorCodeCommunicationNetworkDown:
		return "network-down"
	case ErrorCodeCommunicationRejectBufferOverflow:
		return "reject-buffer-overflow"
	case ErrorCodeCommunicationRejectInconsistentParameters:
		return "reject-inconsistent-parameters"
	case ErrorCodeCommunicationRejectInvalidParameterDataType:
		return "reject-invalid-parameter-data-type"
	case ErrorCodeCommunicationRejectInvalidTag:
		return "reject-invalid-tag"
	case ErrorCodeCommunicationRejectMissingRequiredParameter:
		return "reject-missing-required-parameter"
	case ErrorCodeCommunicationRejectParameterOutOfRange:
		return "reject-parameter-out-of-range"
	case ErrorCodeCommunicationRejectTooManyArguments:
		return "reject-too-many-arguments"
	case ErrorCodeCommunicationRejectUndefinedEnumeration:
		return "reject-undefined-enumeration"
	case ErrorCodeCommunicationRejectUnrecognizedService:
		return "reject-unrecognized-service"
	case ErrorCodeCommunicationRejectProprietary:
		return "reject-proprietary"
	case ErrorCodeCommunicationRejectOther:
		return "reject-other"
	case ErrorCodeCommunicationUnknownDevice:
		return "unknown-device"
	case ErrorCodeCommunicationUnknownRoute:
		return "unknown-route"
	case ErrorCodeCommunicationNotRouterToDnet:
		return "not-router-to-dnet"
	case ErrorCodeCommunicationRouterBusy:
		return "router-busy"
	case ErrorCodeCommunicationUnknownNetworkMessage:
		return "unknown-network-message"
	case ErrorCodeCommunicationMessageTooLong:
		return "message-too-long"
	case ErrorCodeCommunicationSecurityError:
		return "security-error"
	case ErrorCodeCommunicationAddressingError:
		return "addressing-error"
	case ErrorCodeCommunicationWriteBdtFailed:
		return "write-bdt-failed"
	case ErrorCodeCommunicationReadBdtFailed:
		return "read-bdt-failed"
	case ErrorCodeCommunicationRegisterForeignDeviceFailed:
		return "register-foreign-device-failed"
	case ErrorCodeCommunicationReadFdtFailed:
		return "read-fdt-failed"
	case ErrorCodeCommunicationDeleteFdtEntryFailed:
		return "delete-fdt-entry-failed"
	case ErrorCodeCommunicationDistributeBroadcastFailed:
		return "distribute-broadcast-failed"
	case ErrorCodeCommunicationAbortApduTooLong:
		return "abort-apdu-too-long"
	case ErrorCodeCommunicationAbortApplicationExceededReplyTime:
		return "abort-application-exceeded-reply-time"
	case ErrorCodeCommunicationAbortOutOfResources:
		return "abort-out-of-resources"
	case ErrorCodeCommunicationAbortTsmTimeout:
		return "abort-tsm-timeout"
	case ErrorCodeCommunicationAbortWindowSizeOutOfRange:
		return "abort-window-size-out-of-range"
	case ErrorCodeCommunicationAbortInsufficientSecurity:
		return "abort-insufficient-security"
	case ErrorCodeCommunicationAbortSecurityError:
		return "abort-security-error"
	case ErrorCodeCommunicationBvlcFunctionUnknown:
		return "bvlc-function-unknown"
	case ErrorCodeCommunicationBvlcProprietaryFunctionUnknown:
		return "bvlc-proprietary-function-unknown"
	case ErrorCodeCommunicationHeaderEncodingError:
		return "header-encoding-error"
	case ErrorCodeCommunicationHeaderNotUnderstood:
		return "header-not-understood"
	case ErrorCodeCommunicationMessageIncomplete:
		return "message-incomplete"
	case ErrorCodeCommunicationNotABacnetScHub:
		return "not-a-bacnet-sc-hub"
	case ErrorCodeCommunicationPayloadExpected:
		return "payload-expected"
	case ErrorCodeCommunicationUnexpectedData:
		return "unexpected-data"
	case ErrorCodeCommunicationNodeDuplicateVmac:
		return "node-duplicate-vmac"
	case ErrorCodeCommunicationHttpUnexpectedResponseCode:
		return "http-unexpected-response-code"
	case ErrorCodeCommunicationHttpNoUpgrade:
		return "http-no-upgrade"
	case ErrorCodeCommunicationHttpResourceNotLocal:
		return "http-resource-not-local"
	case ErrorCodeCommunicationHttpProxyAuthenticationFailed:
		return "http-proxy-authentication-failed"
	case ErrorCodeCommunicationHttpResponseTimeout:
		return "http-response-timeout"
	case ErrorCodeCommunicationHttpResponseSyntaxError:
		return "http-response-syntax-error"
	case ErrorCodeCommunicationHttpResponseValueError:
		return "http-response-value-error"
	case ErrorCodeCommunicationHttpResponseMissingHeader:
		return "http-response-missing-header"
	case ErrorCodeCommunicationHttpWebsocketHeaderError:
		return "http-websocket-header-error"
	case ErrorCodeCommunicationHttpUpgradeRequired:
		return "http-upgrade-required"
	case ErrorCodeCommunicationHttpUpgradeError:
		return "http-upgrade-error"
	case ErrorCodeCommunicationHttpTemporaryUnavailable:
		return "http-temporary-unavailable"
	case ErrorCodeCommunicationHttpNotAServer:
		return "http-not-a-server"
	case ErrorCodeCommunicationHttpError:
		return "http-error"
	case ErrorCodeCommunicationWebsocketSchemeNotSupported:
		return "websocket-scheme-not-supported"
	case ErrorCodeCommunicationWebsocketUnknownControlMessage:
		return "websocket-unknown-control-message"
	case ErrorCodeCommunicationWebsocketCloseError:
		return "websocket-close-error"
	case ErrorCodeCommunicationWebsocketClosedByPeer:
		return "websocket-closed-by-peer"
	case ErrorCodeCommunicationWebsocketEndpointLeaves:
		return "websocket-endpoint-leaves"
	case ErrorCodeCommunicationWebsocketProtocolError:
		return "websocket-protocol-error"
	case ErrorCodeCommunicationWebsocketDataNotAccepted:
		return "websocket-data-not-accepted"
	case ErrorCodeCommunicationWebsocketClosedAbnormally:
		return "websocket-closed-abnormally"
	case ErrorCodeCommunicationWebsocketDataInconsistent:
		return "websocket-data-inconsistent"
	case ErrorCodeCommunicationWebsocketDataAgainstPolicy:
		return "websocket-data-against-policy"
	case ErrorCodeCommunicationWebsocketFrameTooLong:
		return "websocket-frame-too-long"
	case ErrorCodeCommunicationWebsocketExtensionMissing:
		return "websocket-extension-missing"
	case ErrorCodeCommunicationWebsocketRequestUnavailable:
		return "websocket-request-unavailable"
	case ErrorCodeCommunicationWebsocketError:
		return "websocket-error"
	case ErrorCodeCommunicationTlsClientCertificateError:
		return "tls-client-certificate-error"
	case ErrorCodeCommunicationTlsServerCertificateError:
		return "tls-server-certificate-error"
	case ErrorCodeCommunicationTlsClientAuthenticationFailed:
		return "tls-client-authentication-failed"
	case ErrorCodeCommunicationTlsServerAuthenticationFailed:
		return "tls-server-authentication-failed"
	case ErrorCodeCommunicationTlsClientCertificateExpired:
		return "tls-client-certificate-expired"
	case ErrorCodeCommunicationTlsServerCertificateExpired:
		return "tls-server-certificate-expired"
	case ErrorCodeCommunicationTlsClientCertificateRevoked:
		return "tls-client-certificate-revoked"
	case ErrorCodeCommunicationTlsServerCertificateRevoked:
		return "tls-server-certificate-revoked"
	case ErrorCodeCommunicationTlsError:
		return "tls-error"
	case ErrorCodeCommunicationDnsUnavailable:
		return "dns-unavailable"
	case ErrorCodeCommunicationDnsNameResolutionFailed:
		return "dns-name-resolution-failed"
	case ErrorCodeCommunicationDnsResolverFailure:
		return "dns-resolver-failure"
	case ErrorCodeCommunicationDnsError:
		return "dns-error"
	case ErrorCodeCommunicationTcpConnectTimeout:
		return "tcp-connect-timeout"
	case ErrorCodeCommunicationTcpConnectionRefused:
		return "tcp-connection-refused"
	case ErrorCodeCommunicationTcpClosedByLocal:
		return "tcp-closed-by-local"
	case ErrorCodeCommunicationTcpClosedOther:
		return "tcp-closed-other"
	case ErrorCodeCommunicationTcpError:
		return "tcp-error"
	case ErrorCodeCommunicationIpAddressNotReachable:
		return "ip-address-not-reachable"
	case ErrorCodeCommunicationIpError:
		return "ip-error"
	case ErrorCodeUnknown:
		return "error-code(unknown)"
	default:
		return fmt.Sprintf("error-code(%d)", uint32(e))
	}
}

// Valid reports whether e is a known standard BACnet error code value (clause 18.24.2).
// ErrorCodeUnknown returns false.
func (e ErrorCode) Valid() bool {
	switch e {
	case ErrorCodeOther,
		ErrorCodeDeviceConfigurationInProgress,
		ErrorCodeDeviceBusy,
		ErrorCodeDeviceOperationalProblem,
		ErrorCodeDeviceInconsistentConfiguration,
		ErrorCodeDeviceInternalError,
		ErrorCodeDeviceNotConfigured,
		ErrorCodeObjectDynamicCreationNotSupported,
		ErrorCodeObjectNoObjectsOfSpecifiedType,
		ErrorCodeObjectDeletionNotPermitted,
		ErrorCodeObjectIdentifierAlreadyExists,
		ErrorCodeObjectUnknownObject,
		ErrorCodeObjectUnsupportedObjectType,
		ErrorCodeObjectNoAlarmConfigured,
		ErrorCodeObjectLogBufferFull,
		ErrorCodeObjectBusy,
		ErrorCodeObjectFileFull,
		ErrorCodeObjectInvalidOperationInThisState,
		ErrorCodePropertyInconsistentSelectionCriterion,
		ErrorCodePropertyInvalidDataType,
		ErrorCodePropertyReadAccessDenied,
		ErrorCodePropertyUnknownProperty,
		ErrorCodePropertyValueOutOfRange,
		ErrorCodePropertyWriteAccessDenied,
		ErrorCodePropertyCharacterSetNotSupported,
		ErrorCodePropertyInvalidArrayIndex,
		ErrorCodePropertyNotCovProperty,
		ErrorCodePropertyOptionalFunctionalityNotSupported,
		ErrorCodePropertyDatatypeNotSupported,
		ErrorCodePropertyDuplicateName,
		ErrorCodePropertyDuplicateObjectId,
		ErrorCodePropertyIsNotAnArray,
		ErrorCodePropertyValueNotInitialized,
		ErrorCodePropertyLoggedValuePurged,
		ErrorCodePropertyNoPropertySpecified,
		ErrorCodePropertyNotConfiguredForTriggeredLogging,
		ErrorCodePropertyUnknownFileSize,
		ErrorCodePropertyValueTooLong,
		ErrorCodePropertyDuplicateEntry,
		ErrorCodePropertyInvalidValueInThisState,
		ErrorCodePropertyListItemNotNumbered,
		ErrorCodePropertyListItemNotTimestamped,
		ErrorCodePropertyInvalidDataEncoding,
		ErrorCodeResourcesNoSpaceForObject,
		ErrorCodeResourcesNoSpaceToAddListElement,
		ErrorCodeResourcesNoSpaceToWriteProperty,
		ErrorCodeResourcesOutOfMemory,
		ErrorCodeSecurityPasswordFailure,
		ErrorCodeSecuritySuccess,
		ErrorCodeSecurityAccessDenied,
		ErrorCodeSecurityBadDestinationAddress,
		ErrorCodeSecurityBadDestinationDeviceId,
		ErrorCodeSecurityBadSignature,
		ErrorCodeSecurityBadSourceAddress,
		ErrorCodeSecurityBadTimestamp,
		ErrorCodeSecurityCannotUseKey,
		ErrorCodeSecurityCannotVerifyMessageId,
		ErrorCodeSecurityCorrectKeyRevision,
		ErrorCodeSecurityDestinationDeviceIdRequired,
		ErrorCodeSecurityDuplicateMessage,
		ErrorCodeSecurityEncryptionNotConfigured,
		ErrorCodeSecurityEncryptionRequired,
		ErrorCodeSecurityIncorrectKey,
		ErrorCodeSecurityInvalidKeyData,
		ErrorCodeSecurityKeyUpdateInProgress,
		ErrorCodeSecurityMalformedMessage,
		ErrorCodeSecurityNotKeyServer,
		ErrorCodeSecurityNotConfigured,
		ErrorCodeSecuritySourceSecurityRequired,
		ErrorCodeSecurityTooManyKeys,
		ErrorCodeSecurityUnknownAuthenticationType,
		ErrorCodeSecurityUnknownKey,
		ErrorCodeSecurityUnknownKeyRevision,
		ErrorCodeSecurityUnknownSourceMessage,
		ErrorCodeSecurityCertificateExpired,
		ErrorCodeSecurityCertificateInvalid,
		ErrorCodeSecurityCertificateMalformed,
		ErrorCodeSecurityCertificateRevoked,
		ErrorCodeServicesFileAccessDenied,
		ErrorCodeServicesInconsistentParameters,
		ErrorCodeServicesInvalidFileAccessMethod,
		ErrorCodeServicesInvalidFileStartPosition,
		ErrorCodeServicesInvalidParameterDataType,
		ErrorCodeServicesInvalidTimestamp,
		ErrorCodeServicesMissingRequiredParameter,
		ErrorCodeServicesPropertyIsNotAList,
		ErrorCodeServicesServiceRequestDenied,
		ErrorCodeServicesCovSubscriptionFailed,
		ErrorCodeServicesInvalidConfigurationData,
		ErrorCodeServicesInvalidTag,
		ErrorCodeServicesInvalidEventState,
		ErrorCodeServicesUnknownSubscription,
		ErrorCodeServicesParameterOutOfRange,
		ErrorCodeServicesListElementNotFound,
		ErrorCodeServicesCommunicationDisabled,
		ErrorCodeServicesInconsistentObjectType,
		ErrorCodeVTNoVtSessionsAvailable,
		ErrorCodeVTUnknownVtClass,
		ErrorCodeVTUnknownVtSession,
		ErrorCodeVTVtSessionAlreadyClosed,
		ErrorCodeVTVtSessionTerminationFailure,
		ErrorCodeCommunicationTimeout,
		ErrorCodeCommunicationAbortBufferOverflow,
		ErrorCodeCommunicationAbortInvalidApduInThisState,
		ErrorCodeCommunicationAbortPreemptedByHigherPriorityTask,
		ErrorCodeCommunicationAbortSegmentationNotSupported,
		ErrorCodeCommunicationAbortProprietary,
		ErrorCodeCommunicationAbortOther,
		ErrorCodeCommunicationNetworkDown,
		ErrorCodeCommunicationRejectBufferOverflow,
		ErrorCodeCommunicationRejectInconsistentParameters,
		ErrorCodeCommunicationRejectInvalidParameterDataType,
		ErrorCodeCommunicationRejectInvalidTag,
		ErrorCodeCommunicationRejectMissingRequiredParameter,
		ErrorCodeCommunicationRejectParameterOutOfRange,
		ErrorCodeCommunicationRejectTooManyArguments,
		ErrorCodeCommunicationRejectUndefinedEnumeration,
		ErrorCodeCommunicationRejectUnrecognizedService,
		ErrorCodeCommunicationRejectProprietary,
		ErrorCodeCommunicationRejectOther,
		ErrorCodeCommunicationUnknownDevice,
		ErrorCodeCommunicationUnknownRoute,
		ErrorCodeCommunicationNotRouterToDnet,
		ErrorCodeCommunicationRouterBusy,
		ErrorCodeCommunicationUnknownNetworkMessage,
		ErrorCodeCommunicationMessageTooLong,
		ErrorCodeCommunicationSecurityError,
		ErrorCodeCommunicationAddressingError,
		ErrorCodeCommunicationWriteBdtFailed,
		ErrorCodeCommunicationReadBdtFailed,
		ErrorCodeCommunicationRegisterForeignDeviceFailed,
		ErrorCodeCommunicationReadFdtFailed,
		ErrorCodeCommunicationDeleteFdtEntryFailed,
		ErrorCodeCommunicationDistributeBroadcastFailed,
		ErrorCodeCommunicationAbortApduTooLong,
		ErrorCodeCommunicationAbortApplicationExceededReplyTime,
		ErrorCodeCommunicationAbortOutOfResources,
		ErrorCodeCommunicationAbortTsmTimeout,
		ErrorCodeCommunicationAbortWindowSizeOutOfRange,
		ErrorCodeCommunicationAbortInsufficientSecurity,
		ErrorCodeCommunicationAbortSecurityError,
		ErrorCodeCommunicationBvlcFunctionUnknown,
		ErrorCodeCommunicationBvlcProprietaryFunctionUnknown,
		ErrorCodeCommunicationHeaderEncodingError,
		ErrorCodeCommunicationHeaderNotUnderstood,
		ErrorCodeCommunicationMessageIncomplete,
		ErrorCodeCommunicationNotABacnetScHub,
		ErrorCodeCommunicationPayloadExpected,
		ErrorCodeCommunicationUnexpectedData,
		ErrorCodeCommunicationNodeDuplicateVmac,
		ErrorCodeCommunicationHttpUnexpectedResponseCode,
		ErrorCodeCommunicationHttpNoUpgrade,
		ErrorCodeCommunicationHttpResourceNotLocal,
		ErrorCodeCommunicationHttpProxyAuthenticationFailed,
		ErrorCodeCommunicationHttpResponseTimeout,
		ErrorCodeCommunicationHttpResponseSyntaxError,
		ErrorCodeCommunicationHttpResponseValueError,
		ErrorCodeCommunicationHttpResponseMissingHeader,
		ErrorCodeCommunicationHttpWebsocketHeaderError,
		ErrorCodeCommunicationHttpUpgradeRequired,
		ErrorCodeCommunicationHttpUpgradeError,
		ErrorCodeCommunicationHttpTemporaryUnavailable,
		ErrorCodeCommunicationHttpNotAServer,
		ErrorCodeCommunicationHttpError,
		ErrorCodeCommunicationWebsocketSchemeNotSupported,
		ErrorCodeCommunicationWebsocketUnknownControlMessage,
		ErrorCodeCommunicationWebsocketCloseError,
		ErrorCodeCommunicationWebsocketClosedByPeer,
		ErrorCodeCommunicationWebsocketEndpointLeaves,
		ErrorCodeCommunicationWebsocketProtocolError,
		ErrorCodeCommunicationWebsocketDataNotAccepted,
		ErrorCodeCommunicationWebsocketClosedAbnormally,
		ErrorCodeCommunicationWebsocketDataInconsistent,
		ErrorCodeCommunicationWebsocketDataAgainstPolicy,
		ErrorCodeCommunicationWebsocketFrameTooLong,
		ErrorCodeCommunicationWebsocketExtensionMissing,
		ErrorCodeCommunicationWebsocketRequestUnavailable,
		ErrorCodeCommunicationWebsocketError,
		ErrorCodeCommunicationTlsClientCertificateError,
		ErrorCodeCommunicationTlsServerCertificateError,
		ErrorCodeCommunicationTlsClientAuthenticationFailed,
		ErrorCodeCommunicationTlsServerAuthenticationFailed,
		ErrorCodeCommunicationTlsClientCertificateExpired,
		ErrorCodeCommunicationTlsServerCertificateExpired,
		ErrorCodeCommunicationTlsClientCertificateRevoked,
		ErrorCodeCommunicationTlsServerCertificateRevoked,
		ErrorCodeCommunicationTlsError,
		ErrorCodeCommunicationDnsUnavailable,
		ErrorCodeCommunicationDnsNameResolutionFailed,
		ErrorCodeCommunicationDnsResolverFailure,
		ErrorCodeCommunicationDnsError,
		ErrorCodeCommunicationTcpConnectTimeout,
		ErrorCodeCommunicationTcpConnectionRefused,
		ErrorCodeCommunicationTcpClosedByLocal,
		ErrorCodeCommunicationTcpClosedOther,
		ErrorCodeCommunicationTcpError,
		ErrorCodeCommunicationIpAddressNotReachable,
		ErrorCodeCommunicationIpError:
		return true
	default:
		return false
	}
}

// Class returns the primary BACnet error class for this error code as defined in clause 18.
// Returns ErrorClassUnknown for ErrorCodeUnknown or any unrecognised value.
func (e ErrorCode) Class() ErrorClass {
	switch e {
	// device
	case ErrorCodeDeviceConfigurationInProgress,
		ErrorCodeDeviceBusy,
		ErrorCodeDeviceOperationalProblem,
		ErrorCodeDeviceInconsistentConfiguration,
		ErrorCodeDeviceInternalError,
		ErrorCodeDeviceNotConfigured:
		return ErrorClassDevice
	// object
	case ErrorCodeObjectDynamicCreationNotSupported,
		ErrorCodeObjectNoObjectsOfSpecifiedType,
		ErrorCodeObjectDeletionNotPermitted,
		ErrorCodeObjectIdentifierAlreadyExists,
		ErrorCodeObjectUnknownObject,
		ErrorCodeObjectUnsupportedObjectType,
		ErrorCodeObjectNoAlarmConfigured,
		ErrorCodeObjectLogBufferFull,
		ErrorCodeObjectBusy,
		ErrorCodeObjectFileFull,
		ErrorCodeObjectInvalidOperationInThisState:
		return ErrorClassObject
	// property
	case ErrorCodePropertyInconsistentSelectionCriterion,
		ErrorCodePropertyInvalidDataType,
		ErrorCodePropertyReadAccessDenied,
		ErrorCodePropertyUnknownProperty,
		ErrorCodePropertyValueOutOfRange,
		ErrorCodePropertyWriteAccessDenied,
		ErrorCodePropertyCharacterSetNotSupported,
		ErrorCodePropertyInvalidArrayIndex,
		ErrorCodePropertyNotCovProperty,
		ErrorCodePropertyOptionalFunctionalityNotSupported,
		ErrorCodePropertyDatatypeNotSupported,
		ErrorCodePropertyDuplicateName,
		ErrorCodePropertyDuplicateObjectId,
		ErrorCodePropertyIsNotAnArray,
		ErrorCodePropertyValueNotInitialized,
		ErrorCodePropertyLoggedValuePurged,
		ErrorCodePropertyNoPropertySpecified,
		ErrorCodePropertyNotConfiguredForTriggeredLogging,
		ErrorCodePropertyUnknownFileSize,
		ErrorCodePropertyValueTooLong,
		ErrorCodePropertyDuplicateEntry,
		ErrorCodePropertyInvalidValueInThisState,
		ErrorCodePropertyListItemNotNumbered,
		ErrorCodePropertyListItemNotTimestamped,
		ErrorCodePropertyInvalidDataEncoding:
		return ErrorClassProperty
	// resources
	case ErrorCodeResourcesNoSpaceForObject,
		ErrorCodeResourcesNoSpaceToAddListElement,
		ErrorCodeResourcesNoSpaceToWriteProperty,
		ErrorCodeResourcesOutOfMemory:
		return ErrorClassResources
	// security
	case ErrorCodeSecurityPasswordFailure,
		ErrorCodeSecuritySuccess,
		ErrorCodeSecurityAccessDenied,
		ErrorCodeSecurityBadDestinationAddress,
		ErrorCodeSecurityBadDestinationDeviceId,
		ErrorCodeSecurityBadSignature,
		ErrorCodeSecurityBadSourceAddress,
		ErrorCodeSecurityBadTimestamp,
		ErrorCodeSecurityCannotUseKey,
		ErrorCodeSecurityCannotVerifyMessageId,
		ErrorCodeSecurityCorrectKeyRevision,
		ErrorCodeSecurityDestinationDeviceIdRequired,
		ErrorCodeSecurityDuplicateMessage,
		ErrorCodeSecurityEncryptionNotConfigured,
		ErrorCodeSecurityEncryptionRequired,
		ErrorCodeSecurityIncorrectKey,
		ErrorCodeSecurityInvalidKeyData,
		ErrorCodeSecurityKeyUpdateInProgress,
		ErrorCodeSecurityMalformedMessage,
		ErrorCodeSecurityNotKeyServer,
		ErrorCodeSecurityNotConfigured,
		ErrorCodeSecuritySourceSecurityRequired,
		ErrorCodeSecurityTooManyKeys,
		ErrorCodeSecurityUnknownAuthenticationType,
		ErrorCodeSecurityUnknownKey,
		ErrorCodeSecurityUnknownKeyRevision,
		ErrorCodeSecurityUnknownSourceMessage,
		ErrorCodeSecurityCertificateExpired,
		ErrorCodeSecurityCertificateInvalid,
		ErrorCodeSecurityCertificateMalformed,
		ErrorCodeSecurityCertificateRevoked:
		return ErrorClassSecurity
	// services
	case ErrorCodeServicesFileAccessDenied,
		ErrorCodeServicesInconsistentParameters,
		ErrorCodeServicesInvalidFileAccessMethod,
		ErrorCodeServicesInvalidFileStartPosition,
		ErrorCodeServicesInvalidParameterDataType,
		ErrorCodeServicesInvalidTimestamp,
		ErrorCodeServicesMissingRequiredParameter,
		ErrorCodeServicesPropertyIsNotAList,
		ErrorCodeServicesServiceRequestDenied,
		ErrorCodeServicesCovSubscriptionFailed,
		ErrorCodeServicesInvalidConfigurationData,
		ErrorCodeServicesInvalidTag,
		ErrorCodeServicesInvalidEventState,
		ErrorCodeServicesUnknownSubscription,
		ErrorCodeServicesParameterOutOfRange,
		ErrorCodeServicesListElementNotFound,
		ErrorCodeServicesCommunicationDisabled,
		ErrorCodeServicesInconsistentObjectType:
		return ErrorClassServices
	// vt
	case ErrorCodeVTNoVtSessionsAvailable,
		ErrorCodeVTUnknownVtClass,
		ErrorCodeVTUnknownVtSession,
		ErrorCodeVTVtSessionAlreadyClosed,
		ErrorCodeVTVtSessionTerminationFailure:
		return ErrorClassVT
	// communication
	case ErrorCodeCommunicationTimeout,
		ErrorCodeCommunicationAbortBufferOverflow,
		ErrorCodeCommunicationAbortInvalidApduInThisState,
		ErrorCodeCommunicationAbortPreemptedByHigherPriorityTask,
		ErrorCodeCommunicationAbortSegmentationNotSupported,
		ErrorCodeCommunicationAbortProprietary,
		ErrorCodeCommunicationAbortOther,
		ErrorCodeCommunicationNetworkDown,
		ErrorCodeCommunicationRejectBufferOverflow,
		ErrorCodeCommunicationRejectInconsistentParameters,
		ErrorCodeCommunicationRejectInvalidParameterDataType,
		ErrorCodeCommunicationRejectInvalidTag,
		ErrorCodeCommunicationRejectMissingRequiredParameter,
		ErrorCodeCommunicationRejectParameterOutOfRange,
		ErrorCodeCommunicationRejectTooManyArguments,
		ErrorCodeCommunicationRejectUndefinedEnumeration,
		ErrorCodeCommunicationRejectUnrecognizedService,
		ErrorCodeCommunicationRejectProprietary,
		ErrorCodeCommunicationRejectOther,
		ErrorCodeCommunicationUnknownDevice,
		ErrorCodeCommunicationUnknownRoute,
		ErrorCodeCommunicationNotRouterToDnet,
		ErrorCodeCommunicationRouterBusy,
		ErrorCodeCommunicationUnknownNetworkMessage,
		ErrorCodeCommunicationMessageTooLong,
		ErrorCodeCommunicationSecurityError,
		ErrorCodeCommunicationAddressingError,
		ErrorCodeCommunicationWriteBdtFailed,
		ErrorCodeCommunicationReadBdtFailed,
		ErrorCodeCommunicationRegisterForeignDeviceFailed,
		ErrorCodeCommunicationReadFdtFailed,
		ErrorCodeCommunicationDeleteFdtEntryFailed,
		ErrorCodeCommunicationDistributeBroadcastFailed,
		ErrorCodeCommunicationAbortApduTooLong,
		ErrorCodeCommunicationAbortApplicationExceededReplyTime,
		ErrorCodeCommunicationAbortOutOfResources,
		ErrorCodeCommunicationAbortTsmTimeout,
		ErrorCodeCommunicationAbortWindowSizeOutOfRange,
		ErrorCodeCommunicationAbortInsufficientSecurity,
		ErrorCodeCommunicationAbortSecurityError,
		ErrorCodeCommunicationBvlcFunctionUnknown,
		ErrorCodeCommunicationBvlcProprietaryFunctionUnknown,
		ErrorCodeCommunicationHeaderEncodingError,
		ErrorCodeCommunicationHeaderNotUnderstood,
		ErrorCodeCommunicationMessageIncomplete,
		ErrorCodeCommunicationNotABacnetScHub,
		ErrorCodeCommunicationPayloadExpected,
		ErrorCodeCommunicationUnexpectedData,
		ErrorCodeCommunicationNodeDuplicateVmac,
		ErrorCodeCommunicationHttpUnexpectedResponseCode,
		ErrorCodeCommunicationHttpNoUpgrade,
		ErrorCodeCommunicationHttpResourceNotLocal,
		ErrorCodeCommunicationHttpProxyAuthenticationFailed,
		ErrorCodeCommunicationHttpResponseTimeout,
		ErrorCodeCommunicationHttpResponseSyntaxError,
		ErrorCodeCommunicationHttpResponseValueError,
		ErrorCodeCommunicationHttpResponseMissingHeader,
		ErrorCodeCommunicationHttpWebsocketHeaderError,
		ErrorCodeCommunicationHttpUpgradeRequired,
		ErrorCodeCommunicationHttpUpgradeError,
		ErrorCodeCommunicationHttpTemporaryUnavailable,
		ErrorCodeCommunicationHttpNotAServer,
		ErrorCodeCommunicationHttpError,
		ErrorCodeCommunicationWebsocketSchemeNotSupported,
		ErrorCodeCommunicationWebsocketUnknownControlMessage,
		ErrorCodeCommunicationWebsocketCloseError,
		ErrorCodeCommunicationWebsocketClosedByPeer,
		ErrorCodeCommunicationWebsocketEndpointLeaves,
		ErrorCodeCommunicationWebsocketProtocolError,
		ErrorCodeCommunicationWebsocketDataNotAccepted,
		ErrorCodeCommunicationWebsocketClosedAbnormally,
		ErrorCodeCommunicationWebsocketDataInconsistent,
		ErrorCodeCommunicationWebsocketDataAgainstPolicy,
		ErrorCodeCommunicationWebsocketFrameTooLong,
		ErrorCodeCommunicationWebsocketExtensionMissing,
		ErrorCodeCommunicationWebsocketRequestUnavailable,
		ErrorCodeCommunicationWebsocketError,
		ErrorCodeCommunicationTlsClientCertificateError,
		ErrorCodeCommunicationTlsServerCertificateError,
		ErrorCodeCommunicationTlsClientAuthenticationFailed,
		ErrorCodeCommunicationTlsServerAuthenticationFailed,
		ErrorCodeCommunicationTlsClientCertificateExpired,
		ErrorCodeCommunicationTlsServerCertificateExpired,
		ErrorCodeCommunicationTlsClientCertificateRevoked,
		ErrorCodeCommunicationTlsServerCertificateRevoked,
		ErrorCodeCommunicationTlsError,
		ErrorCodeCommunicationDnsUnavailable,
		ErrorCodeCommunicationDnsNameResolutionFailed,
		ErrorCodeCommunicationDnsResolverFailure,
		ErrorCodeCommunicationDnsError,
		ErrorCodeCommunicationTcpConnectTimeout,
		ErrorCodeCommunicationTcpConnectionRefused,
		ErrorCodeCommunicationTcpClosedByLocal,
		ErrorCodeCommunicationTcpClosedOther,
		ErrorCodeCommunicationTcpError,
		ErrorCodeCommunicationIpAddressNotReachable,
		ErrorCodeCommunicationIpError:
		return ErrorClassCommunication
	default:
		return ErrorClassUnknown
	}
}

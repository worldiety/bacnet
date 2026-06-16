package apdu

import (
	"fmt"

	"go.wdy.de/bacnet"
)

// MaxSegmentsAccepted is the 3-bit field that encodes the maximum number of APDU
// segments a device is willing to accept in a segmented response, per clause 20.1.2.4
// of ANSI/ASHRAE 135-2024.  It is included in the ICI of confirmed request and
// indication primitives.
type MaxSegmentsAccepted uint8

const (
	// MaxSegmentsUnspecified means no segment limit is declared.
	MaxSegmentsUnspecified MaxSegmentsAccepted = 0b000
	// MaxSegments2 limits accepted segments to 2.
	MaxSegments2 MaxSegmentsAccepted = 0b001
	// MaxSegments4 limits accepted segments to 4.
	MaxSegments4 MaxSegmentsAccepted = 0b010
	// MaxSegments8 limits accepted segments to 8.
	MaxSegments8 MaxSegmentsAccepted = 0b011
	// MaxSegments16 limits accepted segments to 16.
	MaxSegments16 MaxSegmentsAccepted = 0b100
	// MaxSegments32 limits accepted segments to 32.
	MaxSegments32 MaxSegmentsAccepted = 0b101
	// MaxSegments64 limits accepted segments to 64.
	MaxSegments64 MaxSegmentsAccepted = 0b110
	// MaxSegmentsMoreThan64 indicates 65 or more segments are accepted.
	MaxSegmentsMoreThan64 MaxSegmentsAccepted = 0b111
)

// Valid reports whether the value is one of the eight defined MaxSegmentsAccepted codes.
func (m MaxSegmentsAccepted) Valid() bool {
	return m <= MaxSegmentsMoreThan64
}

func (m MaxSegmentsAccepted) String() string {
	switch m {
	case MaxSegmentsUnspecified:
		return "unspecified"
	case MaxSegments2:
		return "2"
	case MaxSegments4:
		return "4"
	case MaxSegments8:
		return "8"
	case MaxSegments16:
		return "16"
	case MaxSegments32:
		return "32"
	case MaxSegments64:
		return "64"
	case MaxSegmentsMoreThan64:
		return "more-than-64"
	default:
		return fmt.Sprintf("%d", m)
	}
}

// ConfirmResult indicates the terminal outcome of a confirmed service exchange.
// It is carried in the ICI of the B-X.confirm primitive, per Table 5-1 of
// clause 5.1.1 in ANSI/ASHRAE 135-2024.
type ConfirmResult byte

const (
	// ConfirmResultPositiveAck indicates the peer returned a SimpleACK or ComplexACK.
	ConfirmResultPositiveAck ConfirmResult = iota
	// ConfirmResultError indicates the peer returned an Error APDU.
	ConfirmResultError
	// ConfirmResultReject indicates the peer returned a Reject APDU.
	ConfirmResultReject
	// ConfirmResultAbort indicates the peer returned an Abort APDU.
	ConfirmResultAbort
	// ConfirmResultCannotSend indicates the request could not be sent.
	ConfirmResultCannotSend
	// ConfirmResultUnexpectedPDU indicates an unexpected inbound PDU was received.
	ConfirmResultUnexpectedPDU
	// ConfirmResultSecurityError indicates a security processing failure occurred.
	ConfirmResultSecurityError
)

func (r ConfirmResult) String() string {
	switch r {
	case ConfirmResultPositiveAck:
		return "positive-ack"
	case ConfirmResultError:
		return "error"
	case ConfirmResultReject:
		return "reject"
	case ConfirmResultAbort:
		return "abort"
	case ConfirmResultCannotSend:
		return "cannot-send"
	case ConfirmResultUnexpectedPDU:
		return "unexpected-pdu"
	case ConfirmResultSecurityError:
		return "security-error"
	default:
		return fmt.Sprintf("confirm-result(%d)", r)
	}
}

// ConfirmedResponseType indicates which APDU kind a confirmed server sends
// for a non-segmented terminal response.
type ConfirmedResponseType byte

const (
	// ConfirmedResponseTypeACK selects SimpleACK/ComplexACK based on payload length.
	ConfirmedResponseTypeACK ConfirmedResponseType = iota
	// ConfirmedResponseTypeError sends an Error APDU.
	ConfirmedResponseTypeError
	// ConfirmedResponseTypeReject sends a Reject APDU.
	ConfirmedResponseTypeReject
	// ConfirmedResponseTypeAbort sends an Abort APDU.
	ConfirmedResponseTypeAbort
)

func (r ConfirmedResponseType) String() string {
	switch r {
	case ConfirmedResponseTypeACK:
		return "ack"
	case ConfirmedResponseTypeError:
		return "error"
	case ConfirmedResponseTypeReject:
		return "reject"
	case ConfirmedResponseTypeAbort:
		return "abort"
	default:
		return fmt.Sprintf("confirmed-response-type(%d)", r)
	}
}

// ConfirmedRequestICI holds the Interface Control Information (ICI) for the
// B-X.request service primitive for confirmed services, per Table 5-1 of
// clause 5.1.1 in ANSI/ASHRAE 135-2024.
//
// The user element populates this ICI and submits it to the ASE when initiating
// a confirmed service interaction. The ASE assigns the InvokeID independently
// before encoding and transmitting the PDU.
type ConfirmedRequestICI struct {
	// Destination is the BACnet address of the intended recipient device.
	Destination bacnet.Address

	// MaxAPDULengthAccepted is the maximum APDU byte size this device can accept
	// in a response (encoded in the Confirmed-Request PDU header per clause 20.1.2).
	MaxAPDULengthAccepted MaxApduLengthAccepted

	// SegmentationSupported declares the segmentation capability of the requestor.
	SegmentationSupported SegmentationSupport

	// MaxSegmentsAccepted is the maximum number of APDU segments this device will
	// accept in a segmented response.
	MaxSegmentsAccepted MaxSegmentsAccepted

	// Priority is the network message priority applied when transmitting this request.
	Priority bacnet.NetworkPriority

	// ServiceRequest carries the confirmed service choice and its encoded parameters.
	ServiceRequest ConfirmedRequest
}

// UnconfirmedRequestICI holds the Interface Control Information for the B-X.request
// service primitive for unconfirmed services, per Table 5-2 of clause 5.1.1 in
// ANSI/ASHRAE 135-2024.
//
// The user element populates this ICI and submits it to the ASE when sending an
// unconfirmed service message.  No InvokeID or segmentation parameters are needed
// because no response is expected.
type UnconfirmedRequestICI struct {
	// Destination is the BACnet address of the intended recipient (or broadcast).
	Destination bacnet.Address

	// Priority is the network message priority applied when transmitting this request.
	Priority bacnet.NetworkPriority

	// ServiceRequest carries the unconfirmed service choice and its encoded parameters.
	ServiceRequest UnconfirmedRequest
}

// ConfirmedIndicationICI holds the Interface Control Information for the
// B-X.indication service primitive for confirmed services, per Table 5-1 of
// clause 5.1.1 in ANSI/ASHRAE 135-2024.
//
// The ASE extracts these parameters from the inbound confirmed-request PDU and
// delivers them to the receiving user element alongside the service parameters.
type ConfirmedIndicationICI struct {
	// Source is the BACnet address of the originating device.
	Source bacnet.Address

	// InvokeID is the invoke identifier assigned by the originator's ASE.
	InvokeID InvokeID

	// MaxAPDULengthAccepted is the maximum APDU byte size the originator can accept
	// in the response (extracted from the PDU header).
	MaxAPDULengthAccepted MaxApduLengthAccepted

	// SegmentationSupported declares the segmentation capability of the originator.
	SegmentationSupported SegmentationSupport

	// MaxSegmentsAccepted is the maximum number of segments the originator will accept.
	MaxSegmentsAccepted MaxSegmentsAccepted

	// Priority is the network message priority of the received request.
	Priority bacnet.NetworkPriority

	// DataExpectingReply reports whether the originator expects a confirmed response.
	// This is always true for confirmed service requests.
	DataExpectingReply bool

	// ServiceRequest carries the confirmed service choice and its encoded parameters.
	ServiceRequest ConfirmedRequest
}

// UnconfirmedIndicationICI holds the Interface Control Information for the
// B-X.indication service primitive for unconfirmed services, per Table 5-2 of
// clause 5.1.1 in ANSI/ASHRAE 135-2024.
//
// The ASE extracts these parameters from the inbound unconfirmed-request PDU and
// delivers them to the receiving user element.
type UnconfirmedIndicationICI struct {
	// Source is the BACnet address of the originating device.
	Source bacnet.Address

	// Priority is the network message priority of the received request.
	Priority bacnet.NetworkPriority

	// ServiceRequest carries the unconfirmed service choice and its encoded parameters.
	ServiceRequest UnconfirmedRequest
}

// ConfirmedResponseICI holds the Interface Control Information for the B-X.response
// service primitive, per Table 5-1 of clause 5.1.1 in ANSI/ASHRAE 135-2024.
//
// The receiving user element (server) assembles this ICI after processing a confirmed
// service indication and returns it to its ASE.  The ASE encodes and transmits the
// appropriate ACK PDU to the original requestor.
type ConfirmedResponseICI struct {
	// Destination is the BACnet address of the original requestor.
	Destination bacnet.Address

	// InvokeID matches the invoke identifier from the original confirmed request.
	InvokeID InvokeID

	// SegmentationSupported declares the responder's segmentation capability.
	SegmentationSupported SegmentationSupport

	// ResponseType selects the non-segmented terminal APDU kind.
	// Zero value ACK derives SimpleACK/ComplexACK from ServiceResponse payload.
	ResponseType ConfirmedResponseType

	// ServiceResponse carries the result payload for the confirmed service.
	// An empty Payload causes the ASE to send a SimpleACK; a non-empty Payload
	// causes it to send a ComplexACK.
	ServiceResponse ServiceResult
}

// ConfirmICI holds the Interface Control Information for the B-X.confirm service
// primitive, per Table 5-1 of clause 5.1.1 in ANSI/ASHRAE 135-2024.
//
// The ASE delivers this ICI to the original requesting user element when a terminal
// response APDU (ACK, Error, Reject, or Abort) is received from the peer.
type ConfirmICI struct {
	// InvokeID matches the invoke identifier from the original request.
	InvokeID InvokeID

	// Result indicates the terminal outcome of the confirmed service exchange.
	Result ConfirmResult

	// ServiceResponse carries the response payload. It is non-nil only when
	// Result == ConfirmResultPositiveAck and the peer returned a ComplexACK.
	// For SimpleACK, Error, Reject, and Abort outcomes this field is nil.
	ServiceResponse *ServiceResult
}

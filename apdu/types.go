package apdu

import "fmt"

// PDUType identifies the BACnet APDU PDU type nibble.
type PDUType byte

const (
	PDUTypeConfirmedRequest   PDUType = 0
	PDUTypeUnconfirmedRequest PDUType = 1
	PDUTypeSimpleACK          PDUType = 2
	PDUTypeComplexACK         PDUType = 3
	PDUTypeSegmentACK         PDUType = 4
	PDUTypeError              PDUType = 5
	PDUTypeReject             PDUType = 6
	PDUTypeAbort              PDUType = 7
)

func (p PDUType) String() string {
	switch p {
	case PDUTypeConfirmedRequest:
		return "confirmed-request"
	case PDUTypeUnconfirmedRequest:
		return "unconfirmed-request"
	case PDUTypeSimpleACK:
		return "simple-ack"
	case PDUTypeComplexACK:
		return "complex-ack"
	case PDUTypeSegmentACK:
		return "segment-ack"
	case PDUTypeError:
		return "error"
	case PDUTypeReject:
		return "reject"
	case PDUTypeAbort:
		return "abort"
	default:
		return fmt.Sprintf("pdu-type(%d)", p)
	}
}

// InvokeID identifies a confirmed transaction.
type InvokeID byte

// ServiceChoice identifies an application service.
type ServiceChoice byte

const (
	ServiceChoiceIAm          ServiceChoice = 0
	ServiceChoiceWhoIs        ServiceChoice = 8
	ServiceChoiceReadProperty ServiceChoice = 12
)

func (s ServiceChoice) String() string {
	switch s {
	case ServiceChoiceIAm:
		return "i-am"
	case ServiceChoiceWhoIs:
		return "who-is"
	case ServiceChoiceReadProperty:
		return "read-property"
	default:
		return fmt.Sprintf("service-choice(%d)", s)
	}
}

// ConfirmedRequest carries a confirmed service request payload.
type ConfirmedRequest struct {
	ServiceChoice ServiceChoice
	Payload       []byte
}

// UnconfirmedRequest carries an unconfirmed service request payload.
type UnconfirmedRequest struct {
	ServiceChoice ServiceChoice
	Payload       []byte
}

// ServiceResult is returned by confirmed request handlers.
type ServiceResult struct {
	Payload []byte
}

// ConfirmedAck is returned to a caller waiting on InvokeConfirmed.
type ConfirmedAck struct {
	Type          PDUType
	InvokeID      InvokeID
	ServiceChoice ServiceChoice
	Payload       []byte
}

// InboundAPDU is the normalized result of decoding an inbound APDU frame.
type InboundAPDU struct {
	Type          PDUType
	InvokeID      InvokeID
	ServiceChoice ServiceChoice
	Payload       []byte
}

// OutboundAPDU is the normalized APDU frame passed to an encoder.
type OutboundAPDU struct {
	Type          PDUType
	InvokeID      InvokeID
	ServiceChoice ServiceChoice
	Payload       []byte
}

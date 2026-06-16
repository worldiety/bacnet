package apdu

import (
	"fmt"
	"slices"

	"go.wdy.de/bacnet/common/errors"
)

const (
	confirmedRequestFlagSegmentedMessage          byte = 0x08
	confirmedRequestFlagMoreFollows               byte = 0x04
	confirmedRequestFlagSegmentedResponseAccepted byte = 0x02
)

// inboundAPDU is the internal normalized representation of an inbound APDU.
type inboundAPDU struct {
	Type                      PDUType
	SegmentedMessage          bool
	MoreFollows               bool
	SegmentedResponseAccepted bool
	NegativeAck               bool
	Server                    bool
	MaxSegmentsAccepted       MaxSegmentsAccepted
	MaxAPDULengthAccepted     MaxApduLengthAccepted
	InvokeID                  InvokeID
	// SequenceNumber is the segment sequence number; only meaningful when SegmentedMessage is true.
	SequenceNumber uint8
	// ProposedWindowSize is the segment window size proposed by the sender; only meaningful when SegmentedMessage is true.
	ProposedWindowSize uint8
	// ActualWindowSize is carried by Segment-ACK PDUs.
	ActualWindowSize uint8
	ServiceChoice    ServiceChoice
	Payload          []byte
}

// outboundAPDU is the internal normalized representation of an outbound APDU.
type outboundAPDU struct {
	Type                      PDUType
	SegmentedMessage          bool
	MoreFollows               bool
	SegmentedResponseAccepted bool
	MaxSegmentsAccepted       MaxSegmentsAccepted
	MaxAPDULengthAccepted     MaxApduLengthAccepted
	InvokeID                  InvokeID
	ServiceChoice             ServiceChoice
	Payload                   []byte
	// Fields used for PDUTypeSegmentACK encoding.
	SequenceNumber   uint8
	ActualWindowSize uint8
	NegativeAck      bool
	Server           bool
}

func encodeAPDU(apdu outboundAPDU) ([]byte, error) {
	if apdu.Type > PDUTypeAbort {
		return nil, errors.NewValidationError("pdu type", apdu.Type, ErrInvalidPDUType)
	}

	if apdu.MaxSegmentsAccepted != 0 && !apdu.MaxSegmentsAccepted.Valid() {
		return nil, errors.NewValidationError("max segments accepted", apdu.MaxSegmentsAccepted, ErrEncodeFailure)
	}

	switch apdu.Type {
	case PDUTypeConfirmedRequest:
		if apdu.SegmentedMessage || apdu.MoreFollows {
			return nil, ErrSegmentationNotSupported
		}
		maxAPDUCode, err := maxAPDUCodeForLength(apdu.MaxAPDULengthAccepted)
		if err != nil {
			return nil, err
		}

		out := make([]byte, 4+len(apdu.Payload))
		out[0] = byte(apdu.Type << 4)
		if apdu.SegmentedMessage {
			out[0] |= confirmedRequestFlagSegmentedMessage
		}
		if apdu.MoreFollows {
			out[0] |= confirmedRequestFlagMoreFollows
		}
		if apdu.SegmentedResponseAccepted {
			out[0] |= confirmedRequestFlagSegmentedResponseAccepted
		}
		out[1] = (byte(apdu.MaxSegmentsAccepted) << 4) | uint8(maxAPDUCode)
		out[2] = byte(apdu.InvokeID)
		out[3] = byte(apdu.ServiceChoice)
		copy(out[4:], apdu.Payload)
		return out, nil
	case PDUTypeUnconfirmedRequest:
		out := make([]byte, 2+len(apdu.Payload))
		out[0] = byte(apdu.Type << 4)
		out[1] = byte(apdu.ServiceChoice)
		copy(out[2:], apdu.Payload)
		return out, nil
	case PDUTypeSimpleACK, PDUTypeError:
		out := make([]byte, 3+len(apdu.Payload))
		out[0] = byte(apdu.Type << 4)
		out[1] = byte(apdu.InvokeID)
		out[2] = byte(apdu.ServiceChoice)
		copy(out[3:], apdu.Payload)
		return out, nil
	case PDUTypeComplexACK:
		if !apdu.SegmentedMessage {
			out := make([]byte, 3+len(apdu.Payload))
			out[0] = byte(apdu.Type << 4)
			out[1] = byte(apdu.InvokeID)
			out[2] = byte(apdu.ServiceChoice)
			copy(out[3:], apdu.Payload)
			return out, nil
		}

		baseLen := 4 + len(apdu.Payload)
		payloadOffset := 4
		if apdu.SequenceNumber == 0 {
			baseLen++
			payloadOffset++
		}

		out := make([]byte, baseLen)
		out[0] = byte(apdu.Type << 4)
		out[0] |= confirmedRequestFlagSegmentedMessage
		if apdu.MoreFollows {
			out[0] |= confirmedRequestFlagMoreFollows
		}
		out[1] = byte(apdu.InvokeID)
		out[2] = apdu.SequenceNumber
		out[3] = apdu.ActualWindowSize
		if apdu.SequenceNumber == 0 {
			out[4] = byte(apdu.ServiceChoice)
		}
		copy(out[payloadOffset:], apdu.Payload)
		return out, nil
	case PDUTypeReject, PDUTypeAbort:
		if len(apdu.Payload) > 1 {
			return nil, errors.NewValidationError("payload", len(apdu.Payload), ErrEncodeFailure)
		}

		reason := byte(0)
		if len(apdu.Payload) == 1 {
			reason = apdu.Payload[0]
		}

		out := make([]byte, 3)
		out[0] = byte(apdu.Type << 4)
		if apdu.Type == PDUTypeAbort && apdu.Server {
			out[0] |= 0x01
		}
		out[1] = byte(apdu.InvokeID)
		out[2] = reason
		return out, nil
	case PDUTypeSegmentACK:
		// Segment-ACK wire format (§20.1.6):
		//   Byte 0: (SegmentAck << 4) | negativeAck<<1 | server
		//   Byte 1: InvokeID
		//   Byte 2: SequenceNumber
		//   Byte 3: ActualWindowSize
		out := make([]byte, 4)
		out[0] = byte(PDUTypeSegmentACK << 4)
		if apdu.NegativeAck {
			out[0] |= 0x02
		}
		if apdu.Server {
			out[0] |= 0x01
		}
		out[1] = byte(apdu.InvokeID)
		out[2] = apdu.SequenceNumber
		out[3] = apdu.ActualWindowSize
		return out, nil
	default:
		return nil, errors.NewValidationError("pdu type", apdu.Type, ErrInvalidPDUType)
	}
}

func decodeAPDU(raw []byte) (inboundAPDU, error) {
	if len(raw) < 1 {
		return inboundAPDU{}, fmt.Errorf("%w: raw APDU too short", ErrDecodeFailure)
	}

	pduType := PDUType(raw[0] >> 4)
	if pduType > PDUTypeAbort {
		return inboundAPDU{}, errors.NewValidationError("pdu type", pduType, ErrInvalidPDUType)
	}

	decoded := inboundAPDU{Type: pduType}

	switch pduType {
	case PDUTypeConfirmedRequest:
		if len(raw) < 4 {
			return inboundAPDU{}, fmt.Errorf("%w: confirmed-request APDU too short", ErrDecodeFailure)
		}
		decoded.SegmentedMessage = (raw[0] & confirmedRequestFlagSegmentedMessage) != 0
		decoded.MoreFollows = (raw[0] & confirmedRequestFlagMoreFollows) != 0
		decoded.SegmentedResponseAccepted = (raw[0] & confirmedRequestFlagSegmentedResponseAccepted) != 0

		decoded.MaxSegmentsAccepted = MaxSegmentsAccepted((raw[1] >> 4) & 0x07)
		if !decoded.MaxSegmentsAccepted.Valid() {
			return inboundAPDU{}, errors.NewValidationError("max segments accepted", decoded.MaxSegmentsAccepted, ErrDecodeFailure)
		}

		maxAPDU, err := maxAPDULengthForCode(maxApduLengthCode(raw[1] & 0x0F))
		if err != nil {
			return inboundAPDU{}, err
		}
		decoded.MaxAPDULengthAccepted = maxAPDU
		decoded.InvokeID = InvokeID(raw[2])

		if decoded.SegmentedMessage {
			// Segmented confirmed request: byte[3]=SequenceNumber, byte[4]=ProposedWindowSize.
			// ServiceChoice is only present in the first segment (SequenceNumber==0).
			if len(raw) < 5 {
				return inboundAPDU{}, fmt.Errorf("%w: segmented confirmed-request APDU too short", ErrDecodeFailure)
			}
			decoded.SequenceNumber = raw[3]
			decoded.ProposedWindowSize = raw[4]
			if decoded.SequenceNumber == 0 {
				if len(raw) < 6 {
					return inboundAPDU{}, fmt.Errorf("%w: segmented confirmed-request first segment too short", ErrDecodeFailure)
				}
				decoded.ServiceChoice = ServiceChoice(raw[5])
				decoded.Payload = slices.Clone(raw[6:])
			} else {
				// Continuation segments do not carry a ServiceChoice byte.
				decoded.Payload = slices.Clone(raw[5:])
			}
		} else {
			decoded.ServiceChoice = ServiceChoice(raw[3])
			decoded.Payload = slices.Clone(raw[4:])
		}
		return decoded, nil
	case PDUTypeUnconfirmedRequest:
		if len(raw) < 2 {
			return inboundAPDU{}, fmt.Errorf("%w: unconfirmed-request APDU too short", ErrDecodeFailure)
		}
		decoded.ServiceChoice = ServiceChoice(raw[1])
		decoded.Payload = slices.Clone(raw[2:])
		return decoded, nil
	case PDUTypeSimpleACK, PDUTypeComplexACK, PDUTypeError:
		if pduType == PDUTypeComplexACK {
			decoded.SegmentedMessage = (raw[0] & confirmedRequestFlagSegmentedMessage) != 0
			decoded.MoreFollows = (raw[0] & confirmedRequestFlagMoreFollows) != 0
		}
		if decoded.SegmentedMessage {
			if len(raw) < 4 {
				return inboundAPDU{}, fmt.Errorf("%w: segmented complex-ack APDU too short", ErrDecodeFailure)
			}
			decoded.InvokeID = InvokeID(raw[1])
			decoded.SequenceNumber = raw[2]
			decoded.ProposedWindowSize = raw[3]
			if decoded.SequenceNumber == 0 {
				if len(raw) < 5 {
					return inboundAPDU{}, fmt.Errorf("%w: segmented complex-ack first segment too short", ErrDecodeFailure)
				}
				decoded.ServiceChoice = ServiceChoice(raw[4])
				decoded.Payload = slices.Clone(raw[5:])
			} else {
				decoded.Payload = slices.Clone(raw[4:])
			}
			return decoded, nil
		}
		if len(raw) < 3 {
			return inboundAPDU{}, fmt.Errorf("%w: APDU too short", ErrDecodeFailure)
		}
		decoded.InvokeID = InvokeID(raw[1])
		decoded.ServiceChoice = ServiceChoice(raw[2])
		decoded.Payload = slices.Clone(raw[3:])
		return decoded, nil
	case PDUTypeReject, PDUTypeAbort:
		if len(raw) < 3 {
			return inboundAPDU{}, fmt.Errorf("%w: APDU too short", ErrDecodeFailure)
		}
		if pduType == PDUTypeAbort {
			decoded.Server = (raw[0] & 0x01) != 0
		}
		decoded.InvokeID = InvokeID(raw[1])
		decoded.Payload = slices.Clone(raw[2:3])
		return decoded, nil
	case PDUTypeSegmentACK:
		// Segment-ACK: byte[0] flags, byte[1] invokeID, byte[2] seqNum, byte[3] windowSize.
		if len(raw) < 4 {
			return inboundAPDU{}, fmt.Errorf("%w: segment-ack APDU too short", ErrDecodeFailure)
		}
		decoded.NegativeAck = (raw[0] & 0x02) != 0
		decoded.Server = (raw[0] & 0x01) != 0
		decoded.InvokeID = InvokeID(raw[1])
		decoded.SequenceNumber = raw[2]
		decoded.ActualWindowSize = raw[3]
		decoded.ProposedWindowSize = raw[3]
		return decoded, nil
	default:
		return inboundAPDU{}, errors.NewValidationError("pdu type", pduType, ErrInvalidPDUType)
	}

}

type MaxApduLengthAccepted uint16

const (
	maxApduLengthAccepted50Bytes   MaxApduLengthAccepted = 50
	maxApduLengthAccepted128Bytes  MaxApduLengthAccepted = 128
	maxApduLengthAccepted206Bytes  MaxApduLengthAccepted = 206 // fits into lon talk frame
	maxApduLengthAccepted480Bytes  MaxApduLengthAccepted = 480 // fits into arcnet frame
	maxApduLengthAccepted1024Bytes MaxApduLengthAccepted = 1024
	maxApduLengthAccepted1476Bytes MaxApduLengthAccepted = 1476 // npdu fits into ethernet frame
)

type maxApduLengthCode uint8

const (
	maxAPDULengthAcceptedCode0Bytes    maxApduLengthCode = 0b0000
	maxAPDULengthAcceptedCode128Bytes  maxApduLengthCode = 0b0001
	maxAPDULengthAcceptedCode206Bytes  maxApduLengthCode = 0b0010
	maxAPDULengthAcceptedCode80Bytes   maxApduLengthCode = 0b0011
	maxAPDULengthAcceptedCode1024Bytes maxApduLengthCode = 0b0100
	maxAPDULengthAcceptedCode1476Bytes maxApduLengthCode = 0b00101
)

func maxAPDUCodeForLength(length MaxApduLengthAccepted) (maxApduLengthCode, error) {
	if length <= maxApduLengthAccepted50Bytes {
		return maxAPDULengthAcceptedCode0Bytes, nil
	}

	if length <= maxApduLengthAccepted128Bytes {
		return maxAPDULengthAcceptedCode128Bytes, nil
	}

	if length <= maxApduLengthAccepted206Bytes {
		return maxAPDULengthAcceptedCode206Bytes, nil
	}

	if length <= maxApduLengthAccepted480Bytes {
		return maxAPDULengthAcceptedCode80Bytes, nil
	}

	if length <= maxApduLengthAccepted1024Bytes {
		return maxAPDULengthAcceptedCode1024Bytes, nil
	}

	if length <= maxApduLengthAccepted1476Bytes {
		return maxAPDULengthAcceptedCode1476Bytes, nil
	}

	return 0, errors.NewValidationError("max APDU length accepted", length, ErrEncodeFailure)
}

func maxAPDULengthForCode(code maxApduLengthCode) (MaxApduLengthAccepted, error) {
	switch code {
	case maxAPDULengthAcceptedCode0Bytes:
		return maxApduLengthAccepted50Bytes, nil
	case maxAPDULengthAcceptedCode128Bytes:
		return maxApduLengthAccepted128Bytes, nil
	case maxAPDULengthAcceptedCode206Bytes:
		return maxApduLengthAccepted206Bytes, nil
	case maxAPDULengthAcceptedCode80Bytes:
		return maxApduLengthAccepted480Bytes, nil
	case maxAPDULengthAcceptedCode1024Bytes:
		return maxApduLengthAccepted1024Bytes, nil
	case maxAPDULengthAcceptedCode1476Bytes:
		return maxApduLengthAccepted1476Bytes, nil
	default:
		return 0, errors.NewValidationError("max APDU length code", code, ErrDecodeFailure)
	}
}

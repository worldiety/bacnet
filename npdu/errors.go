package npdu

import "errors"

var (
	// ErrInvalidLength indicates that a buffer or field length is outside the valid range.
	ErrInvalidLength = errors.New("invalid length")

	// ErrInvalidProtocolVersion indicates that the NPDU version byte is not 0x01.
	ErrInvalidProtocolVersion = errors.New("invalid protocol version")

	// ErrReservedBitSet indicates that a reserved NPCI bit (4 or 6) is non-zero.
	ErrReservedBitSet = errors.New("reserved NPCI bit is set")

	// ErrInvalidPriority indicates that the network priority value exceeds 3.
	ErrInvalidPriority = errors.New("invalid network priority")

	// ErrInvalidMessageType indicates that a standard NL message constructor received
	// a proprietary message type code (>= 0x80). Use NewProprietaryNetworkLayerMessage.
	ErrInvalidMessageType = errors.New("invalid message type")

	// ErrProprietaryMessageType indicates that a proprietary NL message constructor
	// received a standard message type code (< 0x80). Use NewNetworkLayerMessage.
	ErrProprietaryMessageType = errors.New("proprietary message type requires vendor ID")

	// ErrEncodeFailure indicates that encoding an NPDU failed.
	ErrEncodeFailure = errors.New("encode failure")

	// ErrDecodeFailure indicates that decoding raw bytes as an NPDU failed.
	ErrDecodeFailure = errors.New("decode failure")
)

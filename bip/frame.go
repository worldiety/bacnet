package bip

import (
	"encoding/binary"
	"net/netip"

	"go.wdy.de/bacnet"
	"go.wdy.de/bacnet/internal/util"
)

// Frame is a decoded Annex J BVLC frame.
type Frame struct {
	Type     BVLCType
	Function BVLCFunctionType
	payload  []byte
}

// NewFrameWithType constructs a BVLC frame for the given BVLC type.
func NewFrameWithType(frameType BVLCType, function BVLCFunctionType, payload []byte) (Frame, error) {
	if !frameType.Valid() {
		return Frame{}, bacnet.NewValidationError("type", frameType, ErrInvalidBVLCType)
	}

	if !function.Valid() {
		return Frame{}, bacnet.NewValidationError("function", function, ErrInvalidFunction)
	}

	if len(payload)+BVLCHeaderLen > 0xFFFF {
		return Frame{}, bacnet.NewValidationError("length", len(payload)+BVLCHeaderLen, ErrInvalidLength)
	}

	return Frame{Type: frameType, Function: function, payload: util.CloneBytes(payload)}, nil
}

// NewFrameForAddress constructs a BVLC frame type from IPv4/IPv6 address family.
func NewFrameForAddress(addr netip.Addr, function BVLCFunctionType, payload []byte) (Frame, error) {
	frameType, err := bvlcTypeForAddress(addr)
	if err != nil {
		return Frame{}, err
	}
	return NewFrameWithType(frameType, function, payload)
}

// DecodeFrame parses a raw BVLC datagram.
func DecodeFrame(raw []byte) (Frame, error) {
	if len(raw) < BVLCHeaderLen {
		return Frame{}, bacnet.NewValidationError("raw", len(raw), ErrFrameTooShort)
	}

	frameType := BVLCType(raw[0])
	if !frameType.Valid() {
		return Frame{}, bacnet.NewValidationError("type", frameType, ErrInvalidBVLCType)
	}

	function := BVLCFunctionType(raw[1])
	if !function.Valid() {
		return Frame{}, bacnet.NewValidationError("function", function, ErrInvalidFunction)
	}

	declared := int(binary.BigEndian.Uint16(raw[2:4]))
	if declared < BVLCHeaderLen || declared != len(raw) {
		return Frame{}, bacnet.NewValidationError("length", declared, ErrInvalidLength)
	}

	return Frame{
		Type:     frameType,
		Function: function,
		payload:  util.CloneBytes(raw[BVLCHeaderLen:]),
	}, nil
}

// Encode serializes a BVLC frame into wire bytes.
func (f Frame) Encode() ([]byte, error) {
	if !f.Type.Valid() {
		return nil, bacnet.NewValidationError("type", f.Type, ErrInvalidBVLCType)
	}
	if !f.Function.Valid() {
		return nil, bacnet.NewValidationError("function", f.Function, ErrInvalidFunction)
	}

	totalLen := BVLCHeaderLen + len(f.payload)
	if totalLen > 0xFFFF {
		return nil, bacnet.NewValidationError("length", totalLen, ErrInvalidLength)
	}

	out := make([]byte, totalLen)
	out[0] = byte(f.Type)
	out[1] = byte(f.Function)
	binary.BigEndian.PutUint16(out[2:4], uint16(totalLen))
	copy(out[BVLCHeaderLen:], f.payload)
	return out, nil
}

// PayloadBytes returns a defensive copy of the frame payload.
func (f Frame) PayloadBytes() []byte {
	return util.CloneBytes(f.payload)
}

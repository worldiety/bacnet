package bip

import (
	"encoding/binary"
	"net/netip"

	"go.wdy.de/bacnet"
)

// Frame is a decoded Annex J BVLC frame.
type Frame struct {
	Type     BVLCType
	Function BVLCFunction
	payload  []byte
}

// NewFrameWithType constructs a BVLC frame for the given BVLC type.
func NewFrameWithType(frameType BVLCType, function BVLCFunction, payload []byte) (Frame, error) {
	if !frameType.Valid() {
		return Frame{}, &bacnet.ValidationError{Field: "type", Value: frameType, Err: ErrInvalidBVLCType}
	}

	if !function.Valid() {
		return Frame{}, &bacnet.ValidationError{Field: "function", Value: function, Err: ErrInvalidFunction}
	}

	if len(payload)+BVLCHeaderLen > 0xFFFF {
		return Frame{}, &bacnet.ValidationError{Field: "length", Value: len(payload) + BVLCHeaderLen, Err: ErrInvalidLength}
	}

	return Frame{Type: frameType, Function: function, payload: cloneBytes(payload)}, nil
}

// NewFrameForAddress constructs a BVLC frame type from IPv4/IPv6 address family.
func NewFrameForAddress(addr netip.Addr, function BVLCFunction, payload []byte) (Frame, error) {
	frameType, err := bvlcTypeForAddress(addr)
	if err != nil {
		return Frame{}, err
	}
	return NewFrameWithType(frameType, function, payload)
}

// DecodeFrame parses a raw BVLC datagram.
func DecodeFrame(raw []byte) (Frame, error) {
	if len(raw) < BVLCHeaderLen {
		return Frame{}, &bacnet.ValidationError{Field: "raw", Value: len(raw), Err: ErrFrameTooShort}
	}

	frameType := BVLCType(raw[0])
	if !frameType.Valid() {
		return Frame{}, &bacnet.ValidationError{Field: "type", Value: frameType, Err: ErrInvalidBVLCType}
	}

	function := BVLCFunction(raw[1])
	if !function.Valid() {
		return Frame{}, &bacnet.ValidationError{Field: "function", Value: function, Err: ErrInvalidFunction}
	}

	declared := int(binary.BigEndian.Uint16(raw[2:4]))
	if declared < BVLCHeaderLen || declared != len(raw) {
		return Frame{}, &bacnet.ValidationError{Field: "length", Value: declared, Err: ErrInvalidLength}
	}

	return Frame{
		Type:     frameType,
		Function: function,
		payload:  cloneBytes(raw[BVLCHeaderLen:]),
	}, nil
}

// Encode serializes a BVLC frame into wire bytes.
func (f Frame) Encode() ([]byte, error) {
	if !f.Type.Valid() {
		return nil, &bacnet.ValidationError{Field: "type", Value: f.Type, Err: ErrInvalidBVLCType}
	}
	if !f.Function.Valid() {
		return nil, &bacnet.ValidationError{Field: "function", Value: f.Function, Err: ErrInvalidFunction}
	}

	totalLen := BVLCHeaderLen + len(f.payload)
	if totalLen > 0xFFFF {
		return nil, &bacnet.ValidationError{Field: "length", Value: totalLen, Err: ErrInvalidLength}
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
	return cloneBytes(f.payload)
}

func cloneBytes(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

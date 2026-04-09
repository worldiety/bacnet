package bip

import "errors"

var (
	ErrInvalidBVLCType      = errors.New("invalid BVLC type")
	ErrInvalidFunction      = errors.New("invalid BVLC function")
	ErrInvalidLength        = errors.New("invalid BVLC length")
	ErrFrameTooShort        = errors.New("BVLC frame too short")
	ErrInvalidIPAddress     = errors.New("invalid ip address")
	ErrNilDatagramConn      = errors.New("nil datagram conn")
	ErrDatagramTooLarge     = errors.New("datagram too large")
	ErrReadFailure          = errors.New("read failure")
	ErrWriteFailure         = errors.New("write failure")
	ErrInvalidResultCode    = errors.New("invalid result code")
	ErrInvalidTTL           = errors.New("invalid ttl")
	ErrInvalidMask          = errors.New("invalid broadcast distribution mask")
	ErrRegistrationRejected = errors.New("bbmd rejected foreign device registration")
)

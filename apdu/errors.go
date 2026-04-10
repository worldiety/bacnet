package apdu

import "errors"

var (
	ErrInvalidASEConfig = errors.New("invalid ASE config")
	ErrNilCodec         = errors.New("codec is required")
	ErrNilTransport     = errors.New("transport is required")
	ErrNilASE           = errors.New("ASE is required")

	ErrInvalidPDUType       = errors.New("invalid PDU type")
	ErrInvalidServiceChoice = errors.New("invalid service choice")

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
	ErrSegmentationNotSupported = errors.New("segmentation required but not supported")

	ErrRemoteError  = errors.New("remote error APDU")
	ErrRemoteReject = errors.New("remote reject APDU")
	ErrRemoteAbort  = errors.New("remote abort APDU")
)

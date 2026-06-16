package encoding

import "errors"

var (
	// ErrEncodeFailure marks a failure while encoding BACnet tag/value bytes.
	ErrEncodeFailure = errors.New("encoding encode failure")
	// ErrDecodeFailure marks a failure while decoding BACnet tag/value bytes.
	ErrDecodeFailure = errors.New("encoding decode failure")
)

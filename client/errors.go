package client

import (
	"errors"
	"fmt"

	"github.com/worldiety/bacnet/apdu"
)

// Describe turns a BACnet client error into an operator-friendly message. It
// recognizes the typed remote errors/rejects/aborts and the timeout and
// segmentation sentinels, falling back to err.Error() for anything else.
func Describe(err error) string {
	if err == nil {
		return ""
	}

	var remoteErr apdu.RemoteErrorAPDU
	if errors.As(err, &remoteErr) {
		return fmt.Sprintf("device returned Error: class=%s code=%s", remoteErr.ErrorClass, remoteErr.ErrorCode)
	}

	var rejectErr apdu.RemoteRejectAPDU
	if errors.As(err, &rejectErr) {
		return fmt.Sprintf("device Rejected the request: %s", rejectErr.RejectReason)
	}

	var abortErr apdu.RemoteAbortAPDU
	if errors.As(err, &abortErr) {
		return fmt.Sprintf("device Aborted the request: %s", abortErr.AbortReason)
	}

	switch {
	case errors.Is(err, apdu.ErrAPDUTimeout):
		return "no response within the invoke timeout (device slow, offline, or wrong address)"
	case errors.Is(err, apdu.ErrSegmentationNotSupported):
		return "response too large: segmentation is not supported (read fewer properties or a single element)"
	default:
		return err.Error()
	}
}

// IsRemoteError reports whether err is a remote Error-PDU from the device (as
// opposed to a transport/timeout failure). During enumeration such errors
// usually mean "this object or property does not exist" and can be skipped
// rather than treated as fatal.
func IsRemoteError(err error) bool {
	var remoteErr apdu.RemoteErrorAPDU
	return errors.As(err, &remoteErr)
}

package main

import (
	"errors"
	"fmt"

	"github.com/worldiety/bacnet/apdu"
)

// describeError turns a BACnet client error into an operator-friendly message.
// It recognizes the typed remote errors/rejects/aborts and the timeout sentinel.
func describeError(err error) string {
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

// isPerObjectError reports whether an error is a benign "this object/property
// does not exist" style remote error, which during enumeration should be
// skipped rather than treated as fatal.
func isPerObjectError(err error) bool {
	var remoteErr apdu.RemoteErrorAPDU
	return errors.As(err, &remoteErr)
}

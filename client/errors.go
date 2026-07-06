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

// rpmUnusable reports whether a whole-request failure from a
// ReadPropertyMultiple attempt should cause the caller to fall back to reading
// each property individually. Falling back both works around devices that do
// not implement RPM and, importantly, isolates a single bad property so the
// remaining (valid) properties still return values. It recognizes:
//
//   - a Reject with reason "unrecognized-service" (the device does not
//     implement ReadPropertyMultiple);
//   - an Error with code "service-request-denied" or the communication
//     "unrecognized-service" code (some devices report unsupported this way);
//   - an Error whose code indicates the request referenced something the object
//     or device does not have — "unknown-property", "unknown-object",
//     "unsupported-object-type" or "invalid-array-index". Many devices reject
//     the entire RPM with such an Error-PDU when one requested property is not
//     applicable (e.g. a network-port property that does not exist for the
//     port's network type); reading per-property lets the good ones succeed and
//     confines the error to the offending property;
//   - an Abort due to buffer-overflow, APDU-too-long or
//     segmentation-not-supported (the response would not fit a single APDU);
//   - ErrSegmentationNotSupported, which this client returns when a device
//     answers with a segmented response (the receive path does not reassemble
//     segments).
//
// Timeouts and other failures are not treated as "unusable" and are surfaced to
// the caller. Per-property errors carried inside a successful RPM-ACK never
// reach here; they are already captured per property.
func rpmUnusable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, apdu.ErrSegmentationNotSupported) {
		return true
	}

	var rejectErr apdu.RemoteRejectAPDU
	if errors.As(err, &rejectErr) {
		return rejectErr.RejectReason == apdu.RejectReasonUnrecognizedService
	}

	var remoteErr apdu.RemoteErrorAPDU
	if errors.As(err, &remoteErr) {
		switch remoteErr.ErrorCode {
		case apdu.ErrorCodeServicesServiceRequestDenied,
			apdu.ErrorCodeCommunicationRejectUnrecognizedService,
			apdu.ErrorCodePropertyUnknownProperty,
			apdu.ErrorCodeObjectUnknownObject,
			apdu.ErrorCodeObjectUnsupportedObjectType,
			apdu.ErrorCodePropertyInvalidArrayIndex:
			return true
		}
		return false
	}

	var abortErr apdu.RemoteAbortAPDU
	if errors.As(err, &abortErr) {
		switch abortErr.AbortReason {
		case apdu.AbortReasonBufferOverflow,
			apdu.AbortReasonAPDUTooLong,
			apdu.AbortReasonSegmentationNotSupported:
			return true
		}
		return false
	}

	return false
}

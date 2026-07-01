package apdu

import (
	"context"
	"fmt"

	"github.com/worldiety/bacnet/common/errors"
	"github.com/worldiety/bacnet/common/netprim"
	bacencoding "github.com/worldiety/bacnet/encoding"
)

// DeviceCommunicationControlEnableDisable selects the communication mode.
type DeviceCommunicationControlEnableDisable uint8

const (
	DeviceCommunicationControlEnable DeviceCommunicationControlEnableDisable = iota
	DeviceCommunicationControlDisable
	DeviceCommunicationControlDisableInitiation
)

// DeviceCommunicationControlRequest is the typed request payload for DeviceCommunicationControl.
type DeviceCommunicationControlRequest struct {
	TimeDurationMinutes *uint16
	EnableDisable       DeviceCommunicationControlEnableDisable
	Password            *string
}

// NewDeviceCommunicationControlRequest constructs a validated DeviceCommunicationControlRequest.
func NewDeviceCommunicationControlRequest(
	timeDurationMinutes *uint16,
	enableDisable DeviceCommunicationControlEnableDisable,
	password *string,
) (DeviceCommunicationControlRequest, error) {
	var pwCopy *string
	if password != nil {
		v := *password
		pwCopy = &v
	}
	req := DeviceCommunicationControlRequest{
		TimeDurationMinutes: timeDurationMinutes,
		EnableDisable:       enableDisable,
		Password:            pwCopy,
	}
	if err := validateDeviceCommunicationControlRequest(req); err != nil {
		return DeviceCommunicationControlRequest{}, err
	}
	return req, nil
}

func validateDeviceCommunicationControlRequest(req DeviceCommunicationControlRequest) error {
	if req.EnableDisable > DeviceCommunicationControlDisableInitiation {
		return errors.NewValidationError("enable disable", req.EnableDisable, ErrEncodeFailure)
	}
	if req.Password != nil {
		if len(*req.Password) == 0 {
			return errors.NewValidationError("password", *req.Password, ErrEncodeFailure)
		}
		if !bacencoding.IsASCIIString(*req.Password) {
			return errors.NewValidationError("password", *req.Password, ErrEncodeFailure)
		}
	}
	return nil
}

func (c *clientImpl) DeviceCommunicationControl(ctx context.Context, dst netprim.Address, req DeviceCommunicationControlRequest) error {
	if err := validateDeviceCommunicationControlRequest(req); err != nil {
		return err
	}

	payload, err := encodeDeviceCommunicationControlRequestPayload(req)
	if err != nil {
		return err
	}

	ackPayload, err := c.InvokeConfirmedRaw(ctx, dst, ServiceChoiceDeviceCommunicationControl, payload)
	if err != nil {
		return classifyRemoteAPDUError(ServiceChoiceDeviceCommunicationControl, err)
	}
	if len(ackPayload) != 0 {
		return fmt.Errorf("%w: device-communication-control expected simple-ack payload to be empty", ErrDecodeFailure)
	}
	return nil
}

func encodeDeviceCommunicationControlRequestPayload(req DeviceCommunicationControlRequest) ([]byte, error) {
	out := make([]byte, 0, 24)
	if req.TimeDurationMinutes != nil {
		out = append(out, bacencoding.EncodeContextPrimitive(0, bacencoding.EncodeUnsigned(uint32(*req.TimeDurationMinutes)))...)
	}
	out = append(out, bacencoding.EncodeContextPrimitive(1, bacencoding.EncodeUnsigned(uint32(req.EnableDisable)))...)
	if req.Password != nil {
		charValue, err := bacencoding.EncodeCharacterStringASCIIValue(*req.Password)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid password character-string: %v", ErrEncodeFailure, err)
		}
		out = append(out, bacencoding.EncodeContextPrimitive(2, charValue)...)
	}
	return out, nil
}

// ReinitializeDeviceState identifies the requested reinitialization behavior.
type ReinitializeDeviceState uint8

const (
	ReinitializeDeviceStateColdStart ReinitializeDeviceState = iota
	ReinitializeDeviceStateWarmStart
	ReinitializeDeviceStateStartBackup
	ReinitializeDeviceStateEndBackup
	ReinitializeDeviceStateStartRestore
	ReinitializeDeviceStateEndRestore
	ReinitializeDeviceStateAbortRestore
)

// ReinitializeDeviceRequest is the typed request payload for ReinitializeDevice.
type ReinitializeDeviceRequest struct {
	State    ReinitializeDeviceState
	Password *string
}

// NewReinitializeDeviceRequest constructs a validated ReinitializeDeviceRequest.
func NewReinitializeDeviceRequest(state ReinitializeDeviceState, password *string) (ReinitializeDeviceRequest, error) {
	var pwCopy *string
	if password != nil {
		v := *password
		pwCopy = &v
	}
	req := ReinitializeDeviceRequest{State: state, Password: pwCopy}
	if err := validateReinitializeDeviceRequest(req); err != nil {
		return ReinitializeDeviceRequest{}, err
	}
	return req, nil
}

func validateReinitializeDeviceRequest(req ReinitializeDeviceRequest) error {
	if req.State > ReinitializeDeviceStateAbortRestore {
		return errors.NewValidationError("state", req.State, ErrEncodeFailure)
	}
	if req.Password != nil {
		if len(*req.Password) == 0 {
			return errors.NewValidationError("password", *req.Password, ErrEncodeFailure)
		}
		if !bacencoding.IsASCIIString(*req.Password) {
			return errors.NewValidationError("password", *req.Password, ErrEncodeFailure)
		}
	}
	return nil
}

func (c *clientImpl) ReinitializeDevice(ctx context.Context, dst netprim.Address, req ReinitializeDeviceRequest) error {
	if err := validateReinitializeDeviceRequest(req); err != nil {
		return err
	}

	payload, err := encodeReinitializeDeviceRequestPayload(req)
	if err != nil {
		return err
	}

	ackPayload, err := c.InvokeConfirmedRaw(ctx, dst, ServiceChoiceReinitializeDevice, payload)
	if err != nil {
		return classifyRemoteAPDUError(ServiceChoiceReinitializeDevice, err)
	}
	if len(ackPayload) != 0 {
		return fmt.Errorf("%w: reinitialize-device expected simple-ack payload to be empty", ErrDecodeFailure)
	}
	return nil
}

func encodeReinitializeDeviceRequestPayload(req ReinitializeDeviceRequest) ([]byte, error) {
	out := make([]byte, 0, 16)
	out = append(out, bacencoding.EncodeContextPrimitive(0, bacencoding.EncodeUnsigned(uint32(req.State)))...)
	if req.Password != nil {
		charValue, err := bacencoding.EncodeCharacterStringASCIIValue(*req.Password)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid password character-string: %v", ErrEncodeFailure, err)
		}
		out = append(out, bacencoding.EncodeContextPrimitive(1, charValue)...)
	}
	return out, nil
}

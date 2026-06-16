package apdu

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

// confirmedServerMachine scaffolds the clause 5.4 confirmed-service state
// machine for a responding application entity.
//
// It models both the unsegmented request→handler→ACK flow and the
// segmented-request receive plus segmented ComplexACK response transmit flow
// (clause 5.4.5), accumulating inbound segment payloads before dispatching to
// the application handler.
type confirmedServerMachine struct {
	// mu guards the confirmed Server Machine's internal state and variables. It must be acquired by "public" functions
	//(exported if this were an exported type, e.g. Handle, SetSegmentTimeout) functions that read or write to these.
	//Non-public functions (aka not-exported even if this were an exported type) may assume they are called with the
	//lock held by the caller and access state and variables without locking. It is the caller's responsibility to make
	//sure this is true.
	mu        sync.Mutex
	state     machineState
	variables confirmedServerTransactionVariables
}

func newConfirmedServerMachineWithConfig(cfg confirmedServerMachineConfig) *confirmedServerMachine {
	if cfg.requestPayloadLength < 0 {
		cfg.requestPayloadLength = 0
	}

	if cfg.preferredWindowSize == 0 {
		cfg.preferredWindowSize = 1
	}

	if cfg.maxSegmentDuplicates == 0 {
		cfg.maxSegmentDuplicates = defaultMaxDuplicateCount
	}

	return &confirmedServerMachine{
		state: machineStateIdle,
		variables: confirmedServerTransactionVariables{
			invokeID:                     cfg.invokeID,
			serviceChoice:                cfg.serviceChoice,
			requesterSegmentation:        cfg.requesterSegmentation,
			requesterMaxSegmentsAccepted: cfg.requesterMaxSegmentsAccepted,
			requesterMaxAPDUSizeAccepted: cfg.requesterMaxAPDUSizeAccepted,
			segmentation:                 cfg.segmentation,
			preferredWindowSize:          cfg.preferredWindowSize,
			maxAPDUSizeAccepted:          cfg.maxAPDUSizeAccepted,
			requestPayloadLength:         cfg.requestPayloadLength,
			responsePayloadLength:        0,
			segmented: segmentedTransactionVariables{
				maxDuplicateCount: cfg.maxSegmentDuplicates,
				maxRetries:        cfg.maxRetries,
			},
		},
	}
}

func (m *confirmedServerMachine) Role() machineRole {
	return machineRoleConfirmedServer
}

func (m *confirmedServerMachine) State() machineState {
	return m.state
}

// SetSegmentTimeout updates the segmented receive deadline owned by the machine.
func (m *confirmedServerMachine) SetSegmentTimeout(deadline time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.variables.segmented.segmentTimeout = deadline
}

// SegmentTimeoutExpired reports whether the machine-owned segmented deadline elapsed.
func (m *confirmedServerMachine) SegmentTimeoutExpired(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	deadline := m.variables.segmented.segmentTimeout
	if deadline.IsZero() {
		return false
	}

	return now.After(deadline) //!now.Before(deadline)
}

// AssembledPayload returns a defensive copy of the fully reassembled request
// payload after all segments have been received. It is only meaningful when
// the machine has transitioned out of machineStateSegmentedRequestReceiving.
func (m *confirmedServerMachine) AssembledPayload() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == machineStateSegmentedRequestReceiving {
		return nil
	}

	return slices.Clone(m.variables.segmentBuffer)
}

// buildOutboundAPDU constructs the outboundAPDU for a response-ready event.
//
// The machine owns this mapping so that the ASE does not need to repeat the
// PDU-type decision; it receives a ready-to-encode struct and calls the codec.
func (m *confirmedServerMachine) buildOutboundAPDU(pduType PDUType, result *ServiceResult) *outboundAPDU {
	var payload []byte
	if result != nil {
		payload = result.Payload
	}

	return &outboundAPDU{
		Type:          pduType,
		Server:        pduType == PDUTypeAbort,
		InvokeID:      m.variables.invokeID,
		ServiceChoice: m.variables.serviceChoice,
		Payload:       payload,
	}
}

// buildSegmentACKAPDU constructs a Segment-ACK outbound APDU.
//
// sequenceNumber is the sequence number of the last segment acknowledged.
// negativeAck signals that the segment was rejected (out-of-order / out-of-window).
func (m *confirmedServerMachine) buildSegmentACKAPDU(sequenceNumber segmentSequenceNumber, negativeAck bool) *outboundAPDU {
	return &outboundAPDU{
		Type:             PDUTypeSegmentACK,
		NegativeAck:      negativeAck,
		Server:           true, // server → client direction
		InvokeID:         m.variables.invokeID,
		SequenceNumber:   uint8(sequenceNumber),
		ActualWindowSize: uint8(m.variables.segmented.actualWindowSize),
	}
}

func (m *confirmedServerMachine) lastContiguousSequenceNumber() segmentSequenceNumber {
	if m.variables.segmented.sequenceNumber == 0 {
		return 0
	}

	return m.variables.segmented.sequenceNumber - 1
}

// AbortReason identifies the reason for an Abort APDU per ANSI/ASHRAE 135-2024.
type AbortReason uint8

const (
	// Abort reason wire codes per ANSI/ASHRAE 135-2024 APDU Abort PDU definition.
	AbortReasonOther AbortReason = 0

	AbortReasonBufferOverflow               AbortReason = 1
	AbortReasonInvalidAPDUInThisState       AbortReason = 2
	AbortReasonPreemptedByHigherPriority    AbortReason = 3
	AbortReasonSegmentationNotSupported     AbortReason = 4
	AbortReasonSecurityError                AbortReason = 5
	AbortReasonInsufficientSecurity         AbortReason = 6
	AbortReasonWindowSizeOutOfRange         AbortReason = 7
	AbortReasonApplicationExceededReplyTime AbortReason = 8
	AbortReasonOutOfResources               AbortReason = 9
	AbortReasonTSMTimeout                   AbortReason = 10
	AbortReasonAPDUTooLong                  AbortReason = 11
)

// Valid reports whether the abort reason is within the known range.
func (r AbortReason) Valid() bool {
	return r <= AbortReasonAPDUTooLong
}

// String returns the BACnet specification name for the abort reason.
func (r AbortReason) String() string {
	switch r {
	case AbortReasonOther:
		return "other"
	case AbortReasonBufferOverflow:
		return "buffer-overflow"
	case AbortReasonInvalidAPDUInThisState:
		return "invalid-apdu-in-this-state"
	case AbortReasonPreemptedByHigherPriority:
		return "preempted-by-higher-priority-task"
	case AbortReasonSegmentationNotSupported:
		return "segmentation-not-supported"
	case AbortReasonSecurityError:
		return "security-error"
	case AbortReasonInsufficientSecurity:
		return "insufficient-security"
	case AbortReasonWindowSizeOutOfRange:
		return "window-size-out-of-range"
	case AbortReasonApplicationExceededReplyTime:
		return "application-exceeded-reply-time"
	case AbortReasonOutOfResources:
		return "out-of-resources"
	case AbortReasonTSMTimeout:
		return "tsm-timeout"
	case AbortReasonAPDUTooLong:
		return "apdu-too-long"
	default:
		return fmt.Sprintf("abort-reason(%d)", r)
	}
}

// RejectReason identifies the reason for a Reject APDU per ANSI/ASHRAE 135-2024.
type RejectReason uint8

const (
	RejectReasonOther                    RejectReason = 0
	RejectReasonBufferOverflow           RejectReason = 1
	RejectReasonInconsistentParameters   RejectReason = 2
	RejectReasonInvalidParameterDataType RejectReason = 3
	RejectReasonInvalidTag               RejectReason = 4
	RejectReasonMissingRequiredParameter RejectReason = 5
	RejectReasonParameterOutOfRange      RejectReason = 6
	RejectReasonTooManyArguments         RejectReason = 7
	RejectReasonUndefinedEnumeration     RejectReason = 8
	RejectReasonUnrecognizedService      RejectReason = 9
)

// Valid reports whether the reject reason is within the known range.
func (r RejectReason) Valid() bool {
	return r <= RejectReasonUnrecognizedService
}

// String returns the BACnet specification name for the reject reason.
func (r RejectReason) String() string {
	switch r {
	case RejectReasonOther:
		return "other"
	case RejectReasonBufferOverflow:
		return "buffer-overflow"
	case RejectReasonInconsistentParameters:
		return "inconsistent-parameters"
	case RejectReasonInvalidParameterDataType:
		return "invalid-parameter-data-type"
	case RejectReasonInvalidTag:
		return "invalid-tag"
	case RejectReasonMissingRequiredParameter:
		return "missing-required-parameter"
	case RejectReasonParameterOutOfRange:
		return "parameter-out-of-range"
	case RejectReasonTooManyArguments:
		return "too-many-arguments"
	case RejectReasonUndefinedEnumeration:
		return "undefined-enumeration"
	case RejectReasonUnrecognizedService:
		return "unrecognized-service"
	default:
		return fmt.Sprintf("reject-reason(%d)", r)
	}
}

func (m *confirmedServerMachine) buildAbortAPDU(reason AbortReason) *outboundAPDU {
	if !reason.Valid() {
		reason = AbortReasonOther
	}

	return &outboundAPDU{
		Type:     PDUTypeAbort,
		InvokeID: m.variables.invokeID,
		Server:   true,
		Payload:  []byte{byte(reason)},
	}
}

func (m *confirmedServerMachine) duplicateSegmentOutput(sequenceNumber segmentSequenceNumber, negativeAck bool) machineOutput {
	if m.variables.segmented.observeDuplicate() {
		m.state = machineStateAborted
		m.setResponseMetadata(PDUTypeAbort, 0)
		return machineOutput{
			action:       machineActionSendAbort,
			OutboundAPDU: m.buildAbortAPDU(AbortReasonInvalidAPDUInThisState),
		}
	}

	return machineOutput{
		action:       machineActionSendSegmentACK,
		OutboundAPDU: m.buildSegmentACKAPDU(sequenceNumber, negativeAck),
	}
}

func (m *confirmedServerMachine) drainContiguousSegmentBuffer() machineOutput {
	consumed := receivedSegmentCount(0)
	lastConsumed := m.variables.segmented.sequenceNumber
	lastSegmentConsumed := false

	for {
		payload, moreFollows, ok := m.variables.segmented.takeBufferedSegment(m.variables.segmented.sequenceNumber)
		if !ok {
			break
		}

		m.variables.segmentBuffer = append(m.variables.segmentBuffer, payload...)
		lastConsumed = m.variables.segmented.sequenceNumber
		m.variables.segmented.lastSequenceNumber = lastConsumed
		m.variables.segmented.sequenceNumber++
		m.variables.receivedInCurrentWindow++
		m.variables.segmented.segmentCount++
		consumed++

		if !moreFollows {
			lastSegmentConsumed = true
			break
		}
	}

	if consumed == 0 {
		return machineOutput{action: machineActionNone}
	}

	m.variables.segmented.resetDuplicateCount()

	if lastSegmentConsumed {
		m.state = machineStateAwaitResponse
		return machineOutput{
			action:       machineActionSendSegmentACK,
			OutboundAPDU: m.buildSegmentACKAPDU(lastConsumed, false),
		}
	}

	if segmentWindowSize(m.variables.receivedInCurrentWindow) >= m.variables.segmented.actualWindowSize {
		m.variables.receivedInCurrentWindow = 0
		m.variables.segmented.initialSequenceNumber = m.variables.segmented.sequenceNumber
		return machineOutput{
			action:       machineActionSendSegmentACK,
			OutboundAPDU: m.buildSegmentACKAPDU(lastConsumed, false),
		}
	}

	return machineOutput{action: machineActionNone}
}

func (m *confirmedServerMachine) setResponseMetadata(pduType PDUType, payloadLen int) {
	m.variables.responsePDUType = pduType
	m.variables.responsePDUTypeSet = true
	m.variables.responsePayloadLength = payloadLen
}

func (m *confirmedServerMachine) resolveResponseKind(in machineInput) (PDUType, machineAction, error) {
	if in.HandlerResponseType != nil {
		switch *in.HandlerResponseType {
		case ConfirmedResponseTypeACK:
			if in.HandlerResult != nil && len(in.HandlerResult.Payload) > 0 {
				return PDUTypeComplexACK, machineActionSendComplexACK, nil
			}
			return PDUTypeSimpleACK, machineActionSendSimpleACK, nil
		case ConfirmedResponseTypeError:
			return PDUTypeError, machineActionSendError, nil
		case ConfirmedResponseTypeReject:
			return PDUTypeReject, machineActionSendReject, nil
		case ConfirmedResponseTypeAbort:
			return PDUTypeAbort, machineActionSendAbort, nil
		default:
			return 0, machineActionNone, invalidStateTransition(m.Role(), m.state, machineEventResponseReady)
		}
	}

	if in.HandlerResult != nil && len(in.HandlerResult.Payload) > 0 {
		return PDUTypeComplexACK, machineActionSendComplexACK, nil
	}

	return PDUTypeSimpleACK, machineActionSendSimpleACK, nil
}

// Handle advances the machine by one event and returns the resulting output.
//
// For response-ready events the caller must supply in.HandlerResult; the
// machine derives SimpleACK vs ComplexACK and constructs the outbound APDU so
// the ASE does not duplicate PDU-shaping logic.
func (m *confirmedServerMachine) Handle(event machineEvent, in machineInput) (machineOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.state {
	case machineStateIdle:
		return m.handleInIdleState(event, in)
	case machineStateSegmentedRequestReceiving:
		return m.handleInSegmentedRequestReceivingState(event, in)
	case machineStateAwaitResponse:
		return m.handleInAwaitResponseState(event, in)
	case machineStateAwaitSegmentACK:
		return m.handleInAwaitSegmentACKState(event, in)
	case machineStateCompleted:
		return m.handleInCompletedState(event, in)
	case machineStateAborted:
		return m.handleInAbortedState(event, in)
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

// handleInIdleState handles events when the machine is in the idle state.
//
// When the inbound confirmed request is segmented (SegmentedMessage=true), the
// machine transitions to machineStateSegmentedRequestReceiving and appends the
// first segment payload to the internal buffer.  A Segment-ACK is returned for
// end-of-window or last-segment cases.  For unsegmented requests the machine
// transitions directly to machineStateAwaitResponse.
func (m *confirmedServerMachine) handleInIdleState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventInboundConfirmedRequest:
		if in.InboundAPDU != nil && in.InboundAPDU.SegmentedMessage {
			if m.variables.segmentation != SegmentationSupportReceive && m.variables.segmentation != SegmentationSupportBoth {
				m.state = machineStateAborted
				m.setResponseMetadata(PDUTypeAbort, 0)
				return machineOutput{
					action: machineActionSendAbort,
					OutboundAPDU: &outboundAPDU{
						Type:     PDUTypeAbort,
						Server:   true,
						InvokeID: m.variables.invokeID,
					},
				}, nil
			}

			if in.InboundAPDU.SequenceNumber != 0 {
				m.state = machineStateAborted
				m.setResponseMetadata(PDUTypeAbort, 0)
				return machineOutput{
					action: machineActionSendAbort,
					OutboundAPDU: &outboundAPDU{
						Type:     PDUTypeAbort,
						Server:   true,
						InvokeID: m.variables.invokeID,
					},
				}, nil
			}

			// First segment of a segmented request (§5.4.5).
			proposed := segmentWindowSize(in.InboundAPDU.ProposedWindowSize)
			if proposed == 0 {
				proposed = 1
			}

			windowSize := minWindowSize(proposed, m.variables.preferredWindowSize)

			m.variables.segmented.proposedWindowSize = proposed
			m.variables.segmented.actualWindowSize = windowSize
			m.variables.segmented.sequenceNumber = 0 // next expected
			m.variables.segmented.initialSequenceNumber = 0
			m.variables.segmented.lastSequenceNumber = 0
			m.variables.segmented.segmentCount = 0
			m.variables.segmented.resetDuplicateCount()
			m.variables.segmented.ensureBufferedMaps()
			m.variables.segmentBuffer = m.variables.segmentBuffer[:0]
			m.variables.receivedInCurrentWindow = 0

			// Expecting more segments; move into state waiting for more segments
			m.state = machineStateSegmentedRequestReceiving
			m.variables.segmented.bufferSegment(segmentSequenceNumber(in.InboundAPDU.SequenceNumber), in.InboundAPDU.Payload, in.InboundAPDU.MoreFollows)
			return m.drainContiguousSegmentBuffer(), nil
		}

		// Unsegmented confirmed request.
		m.state = machineStateAwaitResponse
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func minWindowSize(clientProposed segmentWindowSize, serverPreferred segmentWindowSize) segmentWindowSize {
	if clientProposed == 0 {
		clientProposed = 1
	}

	if serverPreferred == 0 {
		serverPreferred = 1
	}

	if clientProposed < serverPreferred {
		return clientProposed
	}

	return serverPreferred
}

func segmentedComplexACKHeaderLength(seq segmentSequenceNumber) int {
	if seq == 0 {
		return 5
	}

	return 4
}

func (m *confirmedServerMachine) responseAPDULimit() int {
	requesterLimit := m.variables.requesterMaxAPDUSizeAccepted
	localLimit := m.variables.maxAPDUSizeAccepted

	switch {
	case requesterLimit == 0:
		return int(localLimit)
	case localLimit == 0:
		return int(requesterLimit)
	case requesterLimit < localLimit:
		return int(requesterLimit)
	default:
		return int(localLimit)
	}
}

func (m *confirmedServerMachine) localSupportsSegmentedResponse() bool {
	return m.variables.segmentation == SegmentationSupportTransmit || m.variables.segmentation == SegmentationSupportBoth
}

func (m *confirmedServerMachine) requesterAcceptsSegmentedResponse() bool {
	return m.variables.requesterSegmentation == SegmentationSupportReceive || m.variables.requesterSegmentation == SegmentationSupportBoth
}

func (m *confirmedServerMachine) segmentedResponsePayloadLimit(seq segmentSequenceNumber) int {
	return m.responseAPDULimit() - segmentedComplexACKHeaderLength(seq)
}

func (m *confirmedServerMachine) segmentedResponseSegmentCount(payloadLen int) int {
	if payloadLen <= 0 {
		return 0
	}

	firstLimit := m.segmentedResponsePayloadLimit(0)
	if firstLimit <= 0 {
		return 0
	}
	if payloadLen <= firstLimit {
		return 1
	}

	continuationLimit := m.segmentedResponsePayloadLimit(1)
	if continuationLimit <= 0 {
		return 0
	}

	remaining := payloadLen - firstLimit
	return 1 + (remaining+continuationLimit-1)/continuationLimit
}

func maxSegmentsAcceptedLimit(maxAccepted MaxSegmentsAccepted) int {
	switch maxAccepted {
	case MaxSegmentsUnspecified:
		return 0
	case MaxSegments2:
		return 2
	case MaxSegments4:
		return 4
	case MaxSegments8:
		return 8
	case MaxSegments16:
		return 16
	case MaxSegments32:
		return 32
	case MaxSegments64:
		return 64
	case MaxSegmentsMoreThan64:
		return 65
	default:
		return 0
	}
}

func (m *confirmedServerMachine) canSegmentResponse(responseType ConfirmedResponseType, payloadLen int) bool {
	if responseType != ConfirmedResponseTypeACK || payloadLen <= 0 {
		return false
	}
	if !m.localSupportsSegmentedResponse() || !m.requesterAcceptsSegmentedResponse() {
		return false
	}

	segmentCount := m.segmentedResponseSegmentCount(payloadLen)
	acceptedLimit := maxSegmentsAcceptedLimit(m.variables.requesterMaxSegmentsAccepted)
	return segmentCount >= 2 && acceptedLimit > 0 && segmentCount <= acceptedLimit
}

func (m *confirmedServerMachine) negotiatedTransmitWindowSize() segmentWindowSize {
	windowSize := m.variables.preferredWindowSize
	if windowSize == 0 {
		windowSize = 1
	}

	if acceptedLimit := maxSegmentsAcceptedLimit(m.variables.requesterMaxSegmentsAccepted); acceptedLimit > 0 && int(windowSize) > acceptedLimit {
		windowSize = segmentWindowSize(acceptedLimit)
	}

	return windowSize
}

func (m *confirmedServerMachine) normalizeTransmitWindowSize(peerWindow segmentWindowSize) segmentWindowSize {
	if peerWindow == 0 {
		peerWindow = 1
	}

	maxWindow := m.negotiatedTransmitWindowSize()
	if peerWindow < maxWindow {
		return peerWindow
	}

	return maxWindow
}

func (m *confirmedServerMachine) buildSegmentedComplexACKAPDUAt(seq segmentSequenceNumber, start int) (*outboundAPDU, int, bool, error) {
	if start < 0 || start >= len(m.variables.responsePayload) {
		return nil, 0, false, ErrTransactionNotReady
	}

	limit := m.segmentedResponsePayloadLimit(seq)
	if limit <= 0 {
		return nil, 0, false, ErrSegmentationNotSupported
	}

	end := start + limit
	if end > len(m.variables.responsePayload) {
		end = len(m.variables.responsePayload)
	}

	sentAll := end == len(m.variables.responsePayload)
	return &outboundAPDU{
		Type:             PDUTypeComplexACK,
		SegmentedMessage: true,
		MoreFollows:      !sentAll,
		InvokeID:         m.variables.invokeID,
		SequenceNumber:   uint8(seq),
		ActualWindowSize: uint8(m.variables.segmented.actualWindowSize),
		ServiceChoice:    m.variables.serviceChoice,
		Payload:          slices.Clone(m.variables.responsePayload[start:end]),
	}, end, sentAll, nil
}

func (m *confirmedServerMachine) buildCurrentSegmentedComplexACKWindow() ([]*outboundAPDU, error) {
	start := m.variables.responsePayloadOffset
	if start < 0 || start >= len(m.variables.responsePayload) {
		return nil, ErrTransactionNotReady
	}

	windowSize := m.variables.segmented.actualWindowSize
	if windowSize == 0 {
		windowSize = 1
	}

	seq := m.variables.segmented.sequenceNumber
	m.variables.segmented.initialSequenceNumber = seq

	offset := start
	out := make([]*outboundAPDU, 0, int(windowSize))
	for i := segmentWindowSize(0); i < windowSize && offset < len(m.variables.responsePayload); i++ {
		apdu, nextOffset, sentAll, err := m.buildSegmentedComplexACKAPDUAt(seq, offset)
		if err != nil {
			return nil, err
		}

		out = append(out, apdu)
		offset = nextOffset
		m.variables.segmented.lastSequenceNumber = seq
		if sentAll {
			break
		}

		seq++
	}

	if len(out) == 0 {
		return nil, ErrTransactionNotReady
	}

	m.variables.responseNextOffset = offset
	m.variables.segmented.sentAllSegments = offset == len(m.variables.responsePayload)
	m.variables.segmented.sequenceNumber = m.variables.segmented.lastSequenceNumber + 1
	return out, nil
}

func (m *confirmedServerMachine) startSegmentedResponse(result *ServiceResult) (machineOutput, error) {
	if result == nil {
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, machineEventResponseRequiresSegmentation)
	}

	m.variables.responsePayload = slices.Clone(result.Payload)
	m.variables.responsePayloadLength = len(result.Payload)
	m.variables.responsePayloadOffset = 0
	m.variables.responseNextOffset = 0
	m.variables.segmented.sequenceNumber = 0
	m.variables.segmented.initialSequenceNumber = 0
	m.variables.segmented.lastSequenceNumber = 0
	m.variables.segmented.actualWindowSize = m.negotiatedTransmitWindowSize()
	m.variables.segmented.retryCount = 0
	m.variables.segmented.sentAllSegments = false
	m.setResponseMetadata(PDUTypeComplexACK, len(result.Payload))

	out, err := m.buildCurrentSegmentedComplexACKWindow()
	if err != nil {
		return machineOutput{}, err
	}

	m.state = machineStateAwaitSegmentACK
	return machineOutput{action: machineActionSendComplexACK, OutboundAPDU: out[0], OutboundAPDUs: out}, nil
}

// handleInSegmentedRequestReceivingState handles events while the machine is
// accumulating segments for a segmented confirmed request (§5.4.5).
//
// On each in-order segment the payload is appended to the internal buffer.
// A Segment-ACK is issued at the end of each window or when the last segment
// (MoreFollows=false) is received. After the last segment the machine
// transitions to machineStateAwaitResponse so the ASE can dispatch to the
// handler. Any out-of-order segment triggers an immediate negative Segment-ACK
// (NAK) and is not buffered.
func (m *confirmedServerMachine) handleInSegmentedRequestReceivingState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventInboundConfirmedRequest: // starting request, always re-ack
		if in.InboundAPDU == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}

		seg := in.InboundAPDU

		// Re-ACK first segment if retransmitted while still in idle state.
		if seg.SequenceNumber == 0 {
			return m.duplicateSegmentOutput(m.lastContiguousSequenceNumber(), false), nil
		}

		// sequence number != 0 => programming error in ASE code
		return machineOutput{}, fmt.Errorf("unexpected segment sequence number %d in state %s", seg.SequenceNumber, m.state)

	case machineEventInboundSegment:
		if in.InboundAPDU == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}

		seg := in.InboundAPDU
		segSeq := segmentSequenceNumber(seg.SequenceNumber)

		if segSeq == m.variables.segmented.sequenceNumber {
			m.variables.segmented.bufferSegment(segSeq, seg.Payload, seg.MoreFollows)
			return m.drainContiguousSegmentBuffer(), nil
		}

		if m.variables.segmented.DuplicateInWindow(segSeq, m.variables.receivedInCurrentWindow) {
			// Duplicate retransmit from the active window: re-ack last contiguous segment.
			return m.duplicateSegmentOutput(m.lastContiguousSequenceNumber(), false), nil
		}

		// Gap / out-of-window segment: keep transaction open and send NAK.
		return machineOutput{
			action:       machineActionSendSegmentACK,
			OutboundAPDU: m.buildSegmentACKAPDU(m.lastContiguousSequenceNumber(), true),
		}, nil

	case machineEventInboundAbort:
		m.state = machineStateAborted
		return machineOutput{action: machineActionNone}, nil

	case machineEventSegmentTimeout:
		m.state = machineStateAborted
		m.setResponseMetadata(PDUTypeAbort, 0)
		return machineOutput{
			action:       machineActionSendAbort,
			OutboundAPDU: m.buildAbortAPDU(AbortReasonTSMTimeout),
		}, nil

	case machineEventClose:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailClosed}, nil

	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedServerMachine) handleInAwaitResponseState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventResponseReady:
		if in.HandlerResult == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}

		pduType, action, err := m.resolveResponseKind(in)
		if err != nil {
			return machineOutput{}, err
		}

		if m.maxAPDUSizeExceeded(len(in.HandlerResult.Payload)) {
			m.state = machineStateAborted
			m.setResponseMetadata(PDUTypeAbort, 0)
			out := m.buildAbortAPDU(AbortReasonAPDUTooLong)
			return machineOutput{action: machineActionSendAbort, OutboundAPDU: out}, nil
		}

		transition, _ := transitionForConfirmedServerResponseNonSegmentedEvent(event)
		m.state = transition.nextState
		m.setResponseMetadata(pduType, len(in.HandlerResult.Payload))
		out := m.buildOutboundAPDU(pduType, in.HandlerResult)
		return machineOutput{action: action, OutboundAPDU: out}, nil
	case machineEventResponseRequiresSegmentation:
		if _, ok := confirmedServerResponseSegmentedEvents[event]; !ok {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		if in.HandlerResult == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		responseType := ConfirmedResponseTypeACK
		if in.HandlerResponseType != nil {
			responseType = *in.HandlerResponseType
		}
		if m.canSegmentResponse(responseType, len(in.HandlerResult.Payload)) {
			return m.startSegmentedResponse(in.HandlerResult)
		}

		m.state = machineStateAborted
		m.setResponseMetadata(PDUTypeAbort, 0)
		out := m.buildAbortAPDU(AbortReasonSegmentationNotSupported)
		return machineOutput{action: machineActionSendAbort, OutboundAPDU: out}, nil
	case machineEventHandlerError:
		m.state = machineStateAborted
		responseType := ConfirmedResponseTypeAbort
		if in.HandlerResponseType != nil {
			responseType = *in.HandlerResponseType
		}
		pduType, action, err := m.resolveResponseKind(machineInput{HandlerResponseType: &responseType, HandlerResult: in.HandlerResult})
		if err != nil {
			return machineOutput{}, err
		}
		m.setResponseMetadata(pduType, 0)
		out := m.buildOutboundAPDU(pduType, nil)
		return machineOutput{action: action, OutboundAPDU: out}, nil
	case machineEventCannotSend:
		m.state = machineStateAborted
		return machineOutput{action: machineActionDeliverCannotSend}, nil
	case machineEventClose:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailClosed}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedServerMachine) maxAPDUSizeExceeded(payloadLen int) bool {
	limit := MaxApduLengthAccepted(m.responseAPDULimit())
	if limit == 0 {
		return false
	}

	return payloadLen > int(limit)
}

func (m *confirmedServerMachine) handleInAwaitSegmentACKState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventInboundSegmentACK:
		if _, ok := confirmedServerInboundSegmentedEvents[event]; !ok {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		if in.InboundAPDU == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}

		ackSeq := segmentSequenceNumber(in.InboundAPDU.SequenceNumber)
		if in.InboundAPDU.ActualWindowSize != 0 {
			m.variables.segmented.actualWindowSize = m.normalizeTransmitWindowSize(segmentWindowSize(in.InboundAPDU.ActualWindowSize))
		}

		if !in.InboundAPDU.NegativeAck && m.variables.segmented.InWindow(ackSeq, m.variables.segmented.initialSequenceNumber) {
			if m.variables.segmented.sentAllSegments {
				m.state = machineStateCompleted
				return machineOutput{action: machineActionNone}, nil
			}

			m.variables.segmented.retryCount = 0
			m.variables.responsePayloadOffset = m.variables.responseNextOffset
			out, err := m.buildCurrentSegmentedComplexACKWindow()
			if err != nil {
				return machineOutput{}, err
			}
			return machineOutput{action: machineActionSendComplexACK, OutboundAPDU: out[0], OutboundAPDUs: out}, nil
		}

		if m.variables.segmented.retryCount < m.variables.segmented.maxRetries {
			m.variables.segmented.retryCount++
			m.variables.segmented.sequenceNumber = m.variables.segmented.initialSequenceNumber
			out, err := m.buildCurrentSegmentedComplexACKWindow()
			if err != nil {
				return machineOutput{}, err
			}
			return machineOutput{action: machineActionSendComplexACK, OutboundAPDU: out[0], OutboundAPDUs: out}, nil
		}

		m.state = machineStateAborted
		m.setResponseMetadata(PDUTypeAbort, 0)
		return machineOutput{action: machineActionSendAbort, OutboundAPDU: m.buildAbortAPDU(AbortReasonTSMTimeout)}, nil
	case machineEventTimeout:
		if m.variables.segmented.retryCount < m.variables.segmented.maxRetries {
			m.variables.segmented.retryCount++
			m.variables.segmented.sequenceNumber = m.variables.segmented.initialSequenceNumber
			out, err := m.buildCurrentSegmentedComplexACKWindow()
			if err != nil {
				return machineOutput{}, err
			}
			return machineOutput{action: machineActionSendComplexACK, OutboundAPDU: out[0], OutboundAPDUs: out}, nil
		}

		m.state = machineStateAborted
		m.setResponseMetadata(PDUTypeAbort, 0)
		return machineOutput{action: machineActionSendAbort, OutboundAPDU: m.buildAbortAPDU(AbortReasonTSMTimeout)}, nil
	case machineEventCannotSend:
		m.state = machineStateAborted
		return machineOutput{action: machineActionDeliverCannotSend}, nil
	case machineEventClose:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailClosed}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedServerMachine) handleInCompletedState(event machineEvent, _ machineInput) (machineOutput, error) {
	switch event {
	case machineEventCannotSend:
		m.state = machineStateAborted
		return machineOutput{action: machineActionDeliverCannotSend}, nil
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedServerMachine) handleInAbortedState(event machineEvent, _ machineInput) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

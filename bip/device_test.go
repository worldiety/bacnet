package bip

import (
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"

	bacneterrors "go.wdy.de/bacnet/common/errors"
	"go.wdy.de/bacnet/common/netprim"
)

// multiWriteConn extends fakeDatagramConn to capture multiple outbound datagrams
// and serve a pre-loaded queue of inbound responses.
type multiWriteConn struct {
	responses []queuedResponse // inbound: served in order on each ReadFromUDPAddrPort call
	readIdx   int
	readErr   error
	deadline  time.Time

	written []struct {
		data []byte
		addr netip.AddrPort
	}
	writeErr error
}

type queuedResponse struct {
	data []byte
	src  netip.AddrPort
}

func (m *multiWriteConn) ReadFromUDPAddrPort(p []byte) (int, netip.AddrPort, error) {
	if m.readErr != nil {
		return 0, netip.AddrPort{}, m.readErr
	}
	if m.readIdx >= len(m.responses) {
		return 0, netip.AddrPort{}, ErrReadFailure
	}
	response := m.responses[m.readIdx]
	m.readIdx++
	n := copy(p, response.data)
	return n, response.src, nil
}

func (m *multiWriteConn) WriteToUDPAddrPort(p []byte, addr netip.AddrPort) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	d := make([]byte, len(p))
	copy(d, p)
	m.written = append(m.written, struct {
		data []byte
		addr netip.AddrPort
	}{data: d, addr: addr})
	return len(p), nil
}

func (m *multiWriteConn) Close() error { return nil }

func (m *multiWriteConn) SetReadDeadline(t time.Time) error {
	m.deadline = t
	return nil
}

// helpers -----------------------------------------------------------------------

var limitedBroadcast = netip.MustParseAddrPort("255.255.255.255:47808")

func mustDeviceIp4(t *testing.T, conn DatagramConn, ttl TTL) DeviceIp4 {
	t.Helper()
	d, err := NewDeviceIp4(conn, ttl)
	if err != nil {
		t.Fatalf("NewDeviceIp4: %v", err)
	}
	return d
}

func encodedBVLCResult(t *testing.T, code BVLCResultCode) []byte {
	t.Helper()
	r, err := NewBVLCResult(code)
	if err != nil {
		t.Fatalf("NewBVLCResult: %v", err)
	}
	raw, err := r.Encode()
	if err != nil {
		t.Fatalf("BVLCResult.Encode: %v", err)
	}
	return raw
}

// NewDeviceIp4 ------------------------------------------------------------------

func TestNewDeviceIp4(t *testing.T) {
	conn := &multiWriteConn{responses: []queuedResponse{}}
	type inputT struct {
		conn DatagramConn
		ttl  TTL
	}
	type outputT struct {
		device DeviceIp4
		err    error
	}
	tests := []struct {
		name           string
		input          inputT
		expectedOutput outputT
	}{
		{
			"ErrNilDatagramConn",
			inputT{nil, 60},
			outputT{nil, ErrNilDatagramConn},
		},
		{
			"ZeroTTLError",
			inputT{conn, 0},
			outputT{nil, ErrInvalidTTL},
		},
		{
			"Valid",
			inputT{conn, 60},
			outputT{DeviceIp4(&deviceImpl{conn, 60}), nil},
		},
	}

	for i, tt := range tests {
		res, err := NewDeviceIp4(tt.input.conn, tt.input.ttl)
		if !errors.Is(err, tt.expectedOutput.err) {
			t.Fatalf("case %d: err = %v, want %v", i, err, tt.expectedOutput.err)
		}
		if !reflect.DeepEqual(res, tt.expectedOutput.device) {
			t.Fatalf("case %d: result = %v, want %v", i, res, tt.expectedOutput.device)
		}
	}
}

// SendLocalBroadcast ------------------------------------------------------------

func TestSendLocalBroadcastDestination(t *testing.T) {
	conn := &multiWriteConn{}
	d := mustDeviceIp4(t, conn, 60)

	npdu, err := NewOriginalBroadcastNpdu(BVLCTypeBACnetIP, []byte{0x01, 0x20})
	if err != nil {
		t.Fatalf("NewOriginalBroadcastNpdu: %v", err)
	}

	if err := d.SendLocalBroadcast(*npdu); err != nil {
		t.Fatalf("SendLocalBroadcast: %v", err)
	}

	if len(conn.written) != 1 {
		t.Fatalf("written datagrams = %d, want 1", len(conn.written))
	}
	if conn.written[0].addr != limitedBroadcast {
		t.Errorf("dst = %v, want %v", conn.written[0].addr, limitedBroadcast)
	}
}

func TestSendLocalBroadcastWireContents(t *testing.T) {
	conn := &multiWriteConn{}
	d := mustDeviceIp4(t, conn, 60)

	npduPayload := []byte{0x01, 0x20, 0x05}
	npdu, _ := NewOriginalBroadcastNpdu(BVLCTypeBACnetIP, npduPayload)
	want, _ := npdu.Encode()

	if err := d.SendLocalBroadcast(*npdu); err != nil {
		t.Fatalf("SendLocalBroadcast: %v", err)
	}

	got := conn.written[0].data
	if len(got) != len(want) {
		t.Fatalf("wire length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte[%d] = 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestSendLocalBroadcastWriteError(t *testing.T) {
	conn := &multiWriteConn{writeErr: ErrWriteFailure}
	d := mustDeviceIp4(t, conn, 60)

	npdu, _ := NewOriginalBroadcastNpdu(BVLCTypeBACnetIP, []byte{0x01})
	err := d.SendLocalBroadcast(*npdu)
	if !errors.Is(err, ErrWriteFailure) {
		t.Fatalf("err = %v, want %v", err, ErrWriteFailure)
	}
}

// SendUnicast -------------------------------------------------------------------

func TestSendUnicastDestination(t *testing.T) {
	conn := &multiWriteConn{}
	d := mustDeviceIp4(t, conn, 60)

	dst := netip.MustParseAddrPort("192.168.1.20:47808")
	msg, err := NewOriginalUnicastNpdu(BVLCTypeBACnetIP, []byte{0x01, 0x00})
	if err != nil {
		t.Fatalf("NewOriginalUnicastNpdu: %v", err)
	}

	if err := d.SendUnicast(dst, *msg); err != nil {
		t.Fatalf("SendUnicast: %v", err)
	}

	if len(conn.written) != 1 {
		t.Fatalf("written datagrams = %d, want 1", len(conn.written))
	}
	if conn.written[0].addr != dst {
		t.Errorf("dst = %v, want %v", conn.written[0].addr, dst)
	}
}

func TestSendUnicastWireContents(t *testing.T) {
	conn := &multiWriteConn{}
	d := mustDeviceIp4(t, conn, 60)

	npduPayload := []byte{0x01, 0x00, 0xAB}
	msg, _ := NewOriginalUnicastNpdu(BVLCTypeBACnetIP, npduPayload)
	want, _ := msg.Encode()
	dst := netip.MustParseAddrPort("192.168.1.5:47808")

	if err := d.SendUnicast(dst, *msg); err != nil {
		t.Fatalf("SendUnicast: %v", err)
	}

	got := conn.written[0].data
	if len(got) != len(want) {
		t.Fatalf("wire length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte[%d] = 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestSendUnicastWriteError(t *testing.T) {
	conn := &multiWriteConn{writeErr: ErrWriteFailure}
	d := mustDeviceIp4(t, conn, 60)

	msg, _ := NewOriginalUnicastNpdu(BVLCTypeBACnetIP, []byte{0x01})
	err := d.SendUnicast(netip.MustParseAddrPort("10.0.0.1:47808"), *msg)
	if !errors.Is(err, ErrWriteFailure) {
		t.Fatalf("err = %v, want %v", err, ErrWriteFailure)
	}
}

func TestSendUnicastInvalidDst(t *testing.T) {
	tests := []struct {
		name string
		dst  netip.AddrPort
	}{
		{"zero value", netip.AddrPort{}},
		{"ipv6", netip.MustParseAddrPort("[::1]:47808")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &multiWriteConn{}
			d := mustDeviceIp4(t, conn, 60)
			msg, _ := NewOriginalUnicastNpdu(BVLCTypeBACnetIP, []byte{0x01})
			err := d.SendUnicast(tt.dst, *msg)
			if !errors.Is(err, bacneterrors.ErrInvalidIPAddress) {
				t.Fatalf("err = %v, want %v", err, bacneterrors.ErrInvalidIPAddress)
			}
		})
	}
}

// RegisterAsForeignDevice -------------------------------------------------------

var validBBMD = netip.MustParseAddr("192.168.1.1")
var validBBMDPort = netip.AddrPortFrom(validBBMD, netprim.IpDefaultUdpPort)

func TestRegisterAsForeignDeviceSuccess(t *testing.T) {
	conn := &multiWriteConn{
		responses: []queuedResponse{{data: encodedBVLCResult(t, ResultCodeSuccessfulCompletion), src: validBBMDPort}},
	}
	d := mustDeviceIp4(t, conn, 60)

	if err := d.RegisterAsForeignDevice(validBBMD); err != nil {
		t.Fatalf("RegisterAsForeignDevice: %v", err)
	}
}

func TestRegisterAsForeignDeviceSendsCorrectTTL(t *testing.T) {
	wantTTL := TTL(120)
	conn := &multiWriteConn{
		responses: []queuedResponse{{data: encodedBVLCResult(t, ResultCodeSuccessfulCompletion), src: validBBMDPort}},
	}
	d := mustDeviceIp4(t, conn, wantTTL)

	if err := d.RegisterAsForeignDevice(validBBMD); err != nil {
		t.Fatalf("RegisterAsForeignDevice: %v", err)
	}

	if len(conn.written) != 1 {
		t.Fatalf("written datagrams = %d, want 1", len(conn.written))
	}

	// Decode the sent frame and verify the TTL field.
	var req RegisterForeignDevice
	if err := req.Decode(conn.written[0].data); err != nil {
		t.Fatalf("decode RegisterForeignDevice: %v", err)
	}
	if req.TTL() != wantTTL {
		t.Errorf("TTL = %d, want %d", req.TTL(), wantTTL)
	}
}

func TestRegisterAsForeignDeviceSendsToCorrectBBMD(t *testing.T) {
	conn := &multiWriteConn{
		responses: []queuedResponse{{data: encodedBVLCResult(t, ResultCodeSuccessfulCompletion), src: validBBMDPort}},
	}
	d := mustDeviceIp4(t, conn, 60)
	if err := d.RegisterAsForeignDevice(validBBMD); err != nil {
		t.Fatalf("RegisterAsForeignDevice: %v", err)
	}

	wantDst := netip.AddrPortFrom(validBBMD, netprim.IpDefaultUdpPort)
	if conn.written[0].addr != wantDst {
		t.Errorf("dst = %v, want %v", conn.written[0].addr, wantDst)
	}
}

func TestRegisterAsForeignDeviceNakRejected(t *testing.T) {
	conn := &multiWriteConn{
		responses: []queuedResponse{{data: encodedBVLCResult(t, ResultCodeRegisterForeignDeviceNak), src: validBBMDPort}},
	}
	d := mustDeviceIp4(t, conn, 60)

	err := d.RegisterAsForeignDevice(validBBMD)
	if !errors.Is(err, ErrRegistrationRejected) {
		t.Fatalf("err = %v, want %v", err, ErrRegistrationRejected)
	}
}

func TestRegisterAsForeignDeviceWriteError(t *testing.T) {
	conn := &multiWriteConn{writeErr: ErrWriteFailure}
	d := mustDeviceIp4(t, conn, 60)

	err := d.RegisterAsForeignDevice(validBBMD)
	if !errors.Is(err, ErrWriteFailure) {
		t.Fatalf("err = %v, want %v", err, ErrWriteFailure)
	}
}

func TestRegisterAsForeignDeviceReadError(t *testing.T) {
	conn := &multiWriteConn{readErr: ErrReadFailure}
	d := mustDeviceIp4(t, conn, 60)

	err := d.RegisterAsForeignDevice(validBBMD)
	if !errors.Is(err, ErrReadFailure) {
		t.Fatalf("err = %v, want %v", err, ErrReadFailure)
	}
}

func TestRegisterAsForeignDeviceInvalidAddrError(t *testing.T) {
	tests := []struct {
		name string
		addr netip.Addr
	}{
		{"zero value", netip.Addr{}},
		{"ipv6", netip.MustParseAddr("::1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &multiWriteConn{}
			d := mustDeviceIp4(t, conn, 60)
			err := d.RegisterAsForeignDevice(tt.addr)
			if !errors.Is(err, bacneterrors.ErrInvalidIPAddress) {
				t.Fatalf("err = %v, want %v", err, bacneterrors.ErrInvalidIPAddress)
			}
		})
	}
}

func TestRegisterAsForeignDeviceIgnoresUnrelatedSenderWhenDeadlineSupported(t *testing.T) {
	unrelated := netip.MustParseAddrPort("192.168.1.200:47808")
	conn := &multiWriteConn{
		responses: []queuedResponse{
			{data: encodedBVLCResult(t, ResultCodeSuccessfulCompletion), src: unrelated},
			{data: encodedBVLCResult(t, ResultCodeSuccessfulCompletion), src: validBBMDPort},
		},
	}
	d := mustDeviceIp4(t, conn, 60)

	if err := d.RegisterAsForeignDevice(validBBMD); err != nil {
		t.Fatalf("RegisterAsForeignDevice: %v", err)
	}
}

func TestRegisterAsForeignDeviceIgnoresMalformedFrameWhenDeadlineSupported(t *testing.T) {
	conn := &multiWriteConn{
		responses: []queuedResponse{
			{data: []byte{0x81, 0x99, 0x00, 0x04}, src: validBBMDPort},
			{data: encodedBVLCResult(t, ResultCodeSuccessfulCompletion), src: validBBMDPort},
		},
	}
	d := mustDeviceIp4(t, conn, 60)

	if err := d.RegisterAsForeignDevice(validBBMD); err != nil {
		t.Fatalf("RegisterAsForeignDevice: %v", err)
	}
}

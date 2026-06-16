package bip

import (
	"context"
	"fmt"
	"net/netip"

	"go.wdy.de/bacnet"
	"go.wdy.de/bacnet/apdu"
	"go.wdy.de/bacnet/npdu"
)

// Stack bridges the BACnet/IP transport layer and the APDU service element (ASE).
//
// It implements apdu.NPDUTransport so it can be passed directly to apdu.NewASE,
// and it provides a Run method that pumps inbound UDP datagrams into the ASE.
//
// Typical usage:
//
//	conn, _ := bip.NewDatagramConn(localAddr)
//	transport, _ := bip.NewTransport(conn, bip.DefaultMaxDatagramSize)
//	stack, _ := bip.NewStack(transport)
//	ase, _ := apdu.NewASE(cfg, stack)
//	defer ase.Close()
//	go stack.Run(ctx, ase)
type Stack struct {
	transport *Transport
}

// NewStack constructs a Stack around an existing Transport.
// transport must not be nil.
func NewStack(transport *Transport) (*Stack, error) {
	if transport == nil {
		return nil, ErrNilTransport
	}
	return &Stack{transport: transport}, nil
}

// SendNPDU implements apdu.NPDUTransport.
//
// It encodes packet to wire bytes, wraps them in an Original-Unicast-NPDU BVLC
// frame, and sends the datagram to the UDP address derived from dst via
// AddressToAddrPort. dst must be a local-network BACnet/IP address (6-byte MAC).
func (s *Stack) SendNPDU(_ context.Context, dst bacnet.Address, packet npdu.NetworkLayerProtocolDataUnit) error {
	npduBytes, err := packet.Encode()
	if err != nil {
		bacnet.Logger.Error("bip stack encode npdu", "error", err, "dst_network", dst.Network)
		return fmt.Errorf("%w: encode npdu: %v", ErrWriteFailure, err)
	}

	udpDst, err := AddressToAddrPort(dst)
	if err != nil {
		bacnet.Logger.Error("bip stack resolve destination", "error", err, "dst_network", dst.Network)
		return fmt.Errorf("%w: resolve destination: %v", ErrUnsupportedAddress, err)
	}

	frame, err := NewFrameWithType(BVLCTypeBACnetIP, FunctionOriginalUnicastNPDU, npduBytes)
	if err != nil {
		bacnet.Logger.Error("bip stack build unicast frame", "error", err, "dst", udpDst)
		return fmt.Errorf("%w: build original-unicast-npdu frame: %v", ErrWriteFailure, err)
	}

	if err := s.transport.SendFrame(udpDst, frame); err != nil {
		bacnet.Logger.Error("bip stack send frame", "error", err, "dst", udpDst)
		return fmt.Errorf("%w: %v", ErrWriteFailure, err)
	}
	return nil
}

// Run starts the inbound receive loop for the Stack, pumping each arriving BVLC
// frame into ase through OnInboundNPDU.
//
// The loop runs until ctx is cancelled or the underlying transport returns a
// permanent error. When ctx is cancelled, Run closes the transport to unblock
// any pending read and then returns ctx.Err().
//
// Frame-level errors (malformed NPDU, unrecognised BVLC function, etc.) are
// silently skipped so that a single bad datagram does not kill the loop.
//
// ase must not be nil.
func (s *Stack) Run(ctx context.Context, ase apdu.ASE) error {
	if ase == nil {
		return ErrNilASE
	}

	// Close the transport when the context is cancelled so that the blocking
	// ReceiveFrame call returns promptly.
	go func() {
		<-ctx.Done()
		_ = s.transport.Close()
	}()

	for {
		frame, senderAddr, err := s.transport.ReceiveFrame()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			bacnet.Logger.Error("bip stack receive frame", "error", err)
			return fmt.Errorf("%w: %v", ErrReadFailure, err)
		}

		_ = s.dispatchFrame(ctx, ase, frame, senderAddr)
	}
}

// dispatchFrame resolves the logical BACnet source address and NPDU bytes from
// an inbound BVLC frame and calls ase.OnInboundNPDU.
//
// Supported function types:
//   - FunctionOriginalUnicastNPDU   – sender is identified by senderAddr.
//   - FunctionOriginalBroadcastNPDU – sender is identified by senderAddr.
//   - FunctionForwardedNPDU         – sender is the originating device encoded
//     in the BVLC payload; the receiving BBMD's address (senderAddr) is ignored.
//
// All other function types are silently ignored.
func (s *Stack) dispatchFrame(ctx context.Context, ase apdu.ASE, frame Frame, senderAddr netip.AddrPort) error {
	var npduBytes []byte
	var src bacnet.Address

	payload := frame.PayloadBytes()

	switch frame.Function {
	case FunctionOriginalUnicastNPDU, FunctionOriginalBroadcastNPDU:
		addr, err := AddrPortToAddress(senderAddr)
		if err != nil {
			bacnet.Logger.Error("bip stack decode sender address", "error", err, "sender", senderAddr)
			return err
		}
		src = addr
		npduBytes = payload

	case FunctionForwardedNPDU:
		// The first 6 bytes of the payload are the originating device's B/IP address.
		if len(payload) <= 6 {
			bacnet.Logger.Error("bip stack forwarded payload too short", "error", ErrReadFailure, "bytes", len(payload))
			return fmt.Errorf("forwarded-npdu payload too short: %d bytes", len(payload))
		}
		originAddr, err := decodeAddressPortIpV4(payload[:6])
		if err != nil {
			bacnet.Logger.Error("bip stack decode forwarded origin", "error", err)
			return fmt.Errorf("decode forwarded-npdu origin address: %w", err)
		}
		addr, err := AddrPortToAddress(originAddr)
		if err != nil {
			bacnet.Logger.Error("bip stack convert forwarded origin", "error", err, "origin", originAddr)
			return err
		}
		src = addr
		npduBytes = payload[6:]

	default:
		// BBMD control frames and other non-data function types are not relevant
		// to the APDU layer; skip them silently.
		return nil
	}

	var pkt npdu.NetworkLayerProtocolDataUnit
	if err := pkt.Decode(npduBytes); err != nil {
		bacnet.Logger.Error("bip stack decode npdu", "error", err, "bytes", len(npduBytes))
		return fmt.Errorf("decode npdu: %w", err)
	}

	return ase.OnInboundNPDU(ctx, src, pkt)
}

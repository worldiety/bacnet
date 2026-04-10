package npdu

import (
	"encoding/binary"
	"fmt"

	"go.wdy.de/bacnet"
)

// minNpduLen is the minimum wire size of an NPDU (version byte + NPCI byte).
const minNpduLen = 2

// Reserved NPCI bit masks per clause 6.2.2 — bits 4 and 6 must be zero on the wire.
const ()

// NetworkLayerProtocolDataUnit represents a decoded BACnet NPDU per clause 6.2.2 of
// ANSI/ASHRAE 135-2024. The zero value is invalid; use a constructor or Decode.
type NetworkLayerProtocolDataUnit struct {
	// protocolVersion is always bacnet.ProtocolVersion (0x01).
	protocolVersion uint8
	flags           NPCIFieldsFlags

	dnet *UltimateDestinationNetworkNumber
	dlen *UltimateDestinationNetworkNumberMacAddressLength
	dadr UltimateDestinationMacLayerAddress

	snet *OriginalSourceNetworkNumber
	slen *OriginalSourceNetworkNumberMacAddressLength
	sadr OriginalSourceMacLayerAddress

	hopCount    *uint8
	messageType *uint8
	vendorId    *uint16
	apdu        []byte
}

// Valid reports whether the NPDU holds a consistent, encodable state per clause 6.2.2.
// It verifies protocol version, reserved-bit constraints, and field-presence rules.
func (n *NetworkLayerProtocolDataUnit) Valid() bool {
	if n == nil {
		return false
	}
	if n.protocolVersion != bacnet.ProtocolVersion {
		return false
	}

	// Bits 4 and 6 are reserved and must be zero per the standard.
	if n.flags&reservedBit4Mask != 0 || n.flags&reservedBit6Mask != 0 {
		return false
	}

	// Bit 7: network layer message flag.
	msgTypePresent := n.flags&isNetworkLayerMessageMask != 0
	if msgTypePresent {
		if n.messageType == nil {
			return false
		}
		// VendorID is required for proprietary message types (>= 0x80) and
		// must be absent for standard types (< 0x80).
		if *n.messageType >= 0x80 {
			if n.vendorId == nil {
				return false
			}
		} else {
			if n.vendorId != nil {
				return false
			}
		}
	} else {
		if n.messageType != nil || n.vendorId != nil {
			return false
		}
	}

	// Bit 5: destination specifier — DNET, DLEN, DADR, HopCount all present or all absent.
	dstSpecifier := n.flags&destinationSpecifierMask != 0
	if dstSpecifier {
		if n.dnet == nil || n.dlen == nil || n.hopCount == nil {
			return false
		}
		if *n.dlen == 0 {
			// DLEN == 0 means broadcast on DNET; DADR must be absent.
			if n.dadr != nil {
				return false
			}
		} else {
			// DLEN > 0: DADR must be present and exactly DLEN bytes long.
			if n.dadr == nil || len(n.dadr) != int(*n.dlen) {
				return false
			}
		}
	} else {
		if n.dnet != nil || n.dlen != nil || n.dadr != nil || n.hopCount != nil {
			return false
		}
	}

	// Bit 3: source specifier — SNET, SLEN, SADR all present or all absent.
	srcSpecifier := n.flags&sourceSpecifierMask != 0
	if srcSpecifier {
		if n.snet == nil || n.slen == nil {
			return false
		}
		// SLEN must be > 0; source broadcasts are not permitted per the standard.
		if *n.slen == 0 {
			return false
		}
		if n.sadr == nil || len(n.sadr) != int(*n.slen) {
			return false
		}
	} else {
		if n.snet != nil || n.slen != nil || n.sadr != nil {
			return false
		}
	}

	return true
}

// Encode serializes the NPDU to its wire representation per clause 6.2.2.
// Returns ErrEncodeFailure if the NPDU is not Valid().
func (n *NetworkLayerProtocolDataUnit) Encode() ([]byte, error) {
	if !n.Valid() {
		return nil, fmt.Errorf("%w: npdu is not in a valid state", ErrEncodeFailure)
	}

	dstSpecifier := n.flags&destinationSpecifierMask != 0
	srcSpecifier := n.flags&sourceSpecifierMask != 0
	nlMsg := n.flags&isNetworkLayerMessageMask != 0

	// Pre-calculate wire size to avoid reallocations.
	size := 2 // version + NPCI
	if dstSpecifier {
		size += 2 + 1 + len(n.dadr) // DNET + DLEN + DADR
	}
	if srcSpecifier {
		size += 2 + 1 + len(n.sadr) // SNET + SLEN + SADR
	}
	if dstSpecifier {
		size++ // HopCount
	}
	if nlMsg {
		size++ // MessageType
		if *n.messageType >= 0x80 {
			size += 2 // VendorID
		}
	}
	size += len(n.apdu)

	out := make([]byte, size)
	i := 0

	out[i] = n.protocolVersion
	i++
	out[i] = uint8(n.flags)
	i++

	if dstSpecifier {
		binary.BigEndian.PutUint16(out[i:], uint16(*n.dnet))
		i += 2
		out[i] = uint8(*n.dlen)
		i++
		copy(out[i:], n.dadr)
		i += len(n.dadr)
	}

	if srcSpecifier {
		binary.BigEndian.PutUint16(out[i:], uint16(*n.snet))
		i += 2
		out[i] = uint8(*n.slen)
		i++
		copy(out[i:], n.sadr)
		i += len(n.sadr)
	}

	if dstSpecifier {
		out[i] = *n.hopCount
		i++
	}

	if nlMsg {
		out[i] = *n.messageType
		i++
		if *n.messageType >= 0x80 {
			binary.BigEndian.PutUint16(out[i:], *n.vendorId)
			i += 2
		}
	}

	copy(out[i:], n.apdu)
	return out, nil
}

// Decode parses raw wire bytes into the receiver per clause 6.2.2.
// data must contain exactly one complete NPDU. Returns ErrDecodeFailure (or a
// more specific sentinel) on malformed input.
func (n *NetworkLayerProtocolDataUnit) Decode(data []byte) error {
	if n == nil {
		return fmt.Errorf("%w: cannot decode into nil pointer", ErrDecodeFailure)
	}
	if len(data) < minNpduLen {
		return bacnet.NewValidationError("data", len(data), ErrInvalidLength)
	}

	res := NetworkLayerProtocolDataUnit{}
	i := 0

	res.protocolVersion = data[i]
	i++
	if res.protocolVersion != bacnet.ProtocolVersion {
		return bacnet.NewValidationError("protocol version", res.protocolVersion, ErrInvalidProtocolVersion)
	}

	res.flags = NPCIFieldsFlags(data[i])
	i++
	if res.flags&reservedBit4Mask != 0 || res.flags&reservedBit6Mask != 0 {
		return bacnet.NewValidationError("NPCI", res.flags, ErrReservedBitSet)
	}

	dstSpecifier := res.flags&destinationSpecifierMask != 0
	srcSpecifier := res.flags&sourceSpecifierMask != 0
	nlMsg := res.flags&isNetworkLayerMessageMask != 0

	// Destination specifier: DNET (2) + DLEN (1) + DADR (DLEN bytes).
	if dstSpecifier {
		if i+3 > len(data) {
			return fmt.Errorf("%w: truncated DNET/DLEN", ErrDecodeFailure)
		}
		dnet := UltimateDestinationNetworkNumber(binary.BigEndian.Uint16(data[i:]))
		i += 2
		res.dnet = &dnet

		dlen := UltimateDestinationNetworkNumberMacAddressLength(data[i])
		i++
		res.dlen = &dlen

		if dlen > 0 {
			if i+int(dlen) > len(data) {
				return fmt.Errorf("%w: truncated DADR", ErrDecodeFailure)
			}
			res.dadr = cloneBytes(data[i : i+int(dlen)])
			i += int(dlen)
		}
	}

	// Source specifier: SNET (2) + SLEN (1) + SADR (SLEN bytes).
	if srcSpecifier {
		if i+3 > len(data) {
			return fmt.Errorf("%w: truncated SNET/SLEN", ErrDecodeFailure)
		}
		snet := OriginalSourceNetworkNumber(binary.BigEndian.Uint16(data[i:]))
		i += 2
		res.snet = &snet

		slen := OriginalSourceNetworkNumberMacAddressLength(data[i])
		i++
		if slen == 0 {
			return bacnet.NewValidationError("SLEN", slen, ErrInvalidLength)
		}
		res.slen = &slen

		if i+int(slen) > len(data) {
			return fmt.Errorf("%w: truncated SADR", ErrDecodeFailure)
		}
		res.sadr = cloneBytes(data[i : i+int(slen)])
		i += int(slen)
	}

	// HopCount follows the address fields and is present only with a destination specifier.
	if dstSpecifier {
		if i >= len(data) {
			return fmt.Errorf("%w: truncated HopCount", ErrDecodeFailure)
		}
		hopCount := data[i]
		i++
		res.hopCount = &hopCount
	}

	// Network layer message type, and VendorID for proprietary types (>= 0x80).
	if nlMsg {
		if i >= len(data) {
			return fmt.Errorf("%w: truncated MessageType", ErrDecodeFailure)
		}
		mt := data[i]
		i++
		res.messageType = &mt

		if mt >= 0x80 {
			if i+2 > len(data) {
				return fmt.Errorf("%w: truncated VendorID", ErrDecodeFailure)
			}
			vid := binary.BigEndian.Uint16(data[i:])
			i += 2
			res.vendorId = &vid
		}
	}

	// Remaining bytes are the APDU or NL message data.
	if i < len(data) {
		res.apdu = cloneBytes(data[i:])
	}

	*n = res
	return nil
}

// --- Constructors ---

// NewLocalAPDU constructs an NPDU carrying a local (non-routed) BACnet APDU.
// This is the most common form for BACnet/IP devices communicating on the same subnet.
// priority controls the NPCI priority field. expectingReply sets the Expecting-Reply bit.
// apdu must be non-empty.
func NewLocalAPDU(priority bacnet.NetworkPriority, expectingReply bool, apdu []byte) (*NetworkLayerProtocolDataUnit, error) {
	if priority > bacnet.NetworkPriorityLifeSafety {
		return nil, bacnet.NewValidationError("priority", priority, ErrInvalidPriority)
	}
	if len(apdu) == 0 {
		return nil, bacnet.NewValidationError("apdu", len(apdu), ErrInvalidLength)
	}

	flags := NPCIFieldsFlags(priority)
	if expectingReply {
		flags |= expectingReplyMask
	}

	return &NetworkLayerProtocolDataUnit{
		protocolVersion: bacnet.ProtocolVersion,
		flags:           flags,
		apdu:            cloneBytes(apdu),
	}, nil
}

// NewRoutedAPDU constructs an NPDU carrying a BACnet APDU addressed to a remote network.
// dadr may be nil or empty to request a broadcast on dnet (DLEN = 0).
// hopCount is the initial hop count (typically 255). apdu must be non-empty.
func NewRoutedAPDU(
	dnet UltimateDestinationNetworkNumber,
	dadr UltimateDestinationMacLayerAddress,
	hopCount uint8,
	priority bacnet.NetworkPriority,
	expectingReply bool,
	apdu []byte,
) (*NetworkLayerProtocolDataUnit, error) {
	if priority > bacnet.NetworkPriorityLifeSafety {
		return nil, bacnet.NewValidationError("priority", priority, ErrInvalidPriority)
	}
	if len(apdu) == 0 {
		return nil, bacnet.NewValidationError("apdu", len(apdu), ErrInvalidLength)
	}
	if len(dadr) > 255 {
		return nil, bacnet.NewValidationError("dadr", len(dadr), ErrInvalidLength)
	}

	dlen := UltimateDestinationNetworkNumberMacAddressLength(len(dadr))
	var dadrCopy UltimateDestinationMacLayerAddress
	if len(dadr) > 0 {
		dadrCopy = cloneBytes(dadr)
	}

	flags := NPCIFieldsFlags(priority) | destinationSpecifierMask
	if expectingReply {
		flags |= expectingReplyMask
	}

	return &NetworkLayerProtocolDataUnit{
		protocolVersion: bacnet.ProtocolVersion,
		flags:           flags,
		dnet:            &dnet,
		dlen:            &dlen,
		dadr:            dadrCopy,
		hopCount:        &hopCount,
		apdu:            cloneBytes(apdu),
	}, nil
}

// NewNetworkLayerMessage constructs an NPDU carrying a standard network layer message
// (messageType < 0x80). For proprietary types use NewProprietaryNetworkLayerMessage.
// data may be nil for message types that carry no payload.
func NewNetworkLayerMessage(messageType uint8, data []byte, priority bacnet.NetworkPriority) (*NetworkLayerProtocolDataUnit, error) {
	if messageType >= 0x80 {
		return nil, bacnet.NewValidationError("message type", messageType, ErrProprietaryMessageType)
	}
	if priority > bacnet.NetworkPriorityLifeSafety {
		return nil, bacnet.NewValidationError("priority", priority, ErrInvalidPriority)
	}

	mt := messageType
	return &NetworkLayerProtocolDataUnit{
		protocolVersion: bacnet.ProtocolVersion,
		flags:           NPCIFieldsFlags(priority) | isNetworkLayerMessageMask,
		messageType:     &mt,
		apdu:            cloneBytes(data),
	}, nil
}

// NewProprietaryNetworkLayerMessage constructs an NPDU carrying a proprietary network
// layer message (messageType >= 0x80). vendorID identifies the originating vendor.
// data may be nil.
func NewProprietaryNetworkLayerMessage(messageType uint8, vendorID uint16, data []byte, priority bacnet.NetworkPriority) (*NetworkLayerProtocolDataUnit, error) {
	if messageType < 0x80 {
		return nil, bacnet.NewValidationError("message type", messageType, ErrInvalidMessageType)
	}
	if priority > bacnet.NetworkPriorityLifeSafety {
		return nil, bacnet.NewValidationError("priority", priority, ErrInvalidPriority)
	}

	mt := messageType
	vid := vendorID
	return &NetworkLayerProtocolDataUnit{
		protocolVersion: bacnet.ProtocolVersion,
		flags:           NPCIFieldsFlags(priority) | isNetworkLayerMessageMask,
		messageType:     &mt,
		vendorId:        &vid,
		apdu:            cloneBytes(data),
	}, nil
}

// --- Accessors ---

// Version returns the protocol version byte (always bacnet.ProtocolVersion for valid NPDUs).
func (n *NetworkLayerProtocolDataUnit) Version() uint8 { return n.protocolVersion }

// Flags returns the raw NPCI control byte.
func (n *NetworkLayerProtocolDataUnit) Flags() NPCIFieldsFlags { return n.flags }

// Priority returns the 2-bit network priority from the NPCI.
func (n *NetworkLayerProtocolDataUnit) Priority() bacnet.NetworkPriority {
	return bacnet.NetworkPriority(n.flags & networkPriorityMask)
}

// IsExpectingReply reports whether the Expecting-Reply bit is set in the NPCI.
func (n *NetworkLayerProtocolDataUnit) IsExpectingReply() bool {
	return n.flags&expectingReplyMask != 0
}

// IsNetworkLayerMessage reports whether the NPDU carries a network layer message
// rather than a BACnet APDU.
func (n *NetworkLayerProtocolDataUnit) IsNetworkLayerMessage() bool {
	return n.flags&isNetworkLayerMessageMask != 0
}

// HasDestinationSpecifier reports whether DNET/DLEN/DADR/HopCount fields are present.
func (n *NetworkLayerProtocolDataUnit) HasDestinationSpecifier() bool {
	return n.flags&destinationSpecifierMask != 0
}

// HasSourceSpecifier reports whether SNET/SLEN/SADR fields are present.
func (n *NetworkLayerProtocolDataUnit) HasSourceSpecifier() bool {
	return n.flags&sourceSpecifierMask != 0
}

// DNET returns the destination network number, or nil if the destination specifier is absent.
func (n *NetworkLayerProtocolDataUnit) DNET() *UltimateDestinationNetworkNumber {
	if n.dnet == nil {
		return nil
	}
	v := *n.dnet
	return &v
}

// DADR returns a defensive copy of the destination MAC-layer address.
// Returns nil when the destination specifier is absent or DLEN is 0 (broadcast on DNET).
func (n *NetworkLayerProtocolDataUnit) DADR() UltimateDestinationMacLayerAddress {
	if n.dadr == nil {
		return nil
	}
	return cloneBytes(n.dadr)
}

// HopCount returns the hop count, or nil if the destination specifier is absent.
func (n *NetworkLayerProtocolDataUnit) HopCount() *uint8 {
	if n.hopCount == nil {
		return nil
	}
	v := *n.hopCount
	return &v
}

// SNET returns the source network number, or nil if the source specifier is absent.
func (n *NetworkLayerProtocolDataUnit) SNET() *OriginalSourceNetworkNumber {
	if n.snet == nil {
		return nil
	}
	v := *n.snet
	return &v
}

// SADR returns a defensive copy of the source MAC-layer address.
// Returns nil when the source specifier is absent.
func (n *NetworkLayerProtocolDataUnit) SADR() OriginalSourceMacLayerAddress {
	if n.sadr == nil {
		return nil
	}
	return cloneBytes(n.sadr)
}

// MessageType returns the network-layer message type byte, or nil if this NPDU
// does not carry a network layer message.
func (n *NetworkLayerProtocolDataUnit) MessageType() *uint8 {
	if n.messageType == nil {
		return nil
	}
	v := *n.messageType
	return &v
}

// VendorID returns the vendor ID for proprietary network layer messages, or nil if absent.
func (n *NetworkLayerProtocolDataUnit) VendorID() *uint16 {
	if n.vendorId == nil {
		return nil
	}
	v := *n.vendorId
	return &v
}

// APDUBytes returns a defensive copy of the APDU or network-layer-message data payload.
func (n *NetworkLayerProtocolDataUnit) APDUBytes() []byte {
	if n.apdu == nil {
		return nil
	}
	return cloneBytes(n.apdu)
}

// --- Types and constants ---

// NPCIFieldsFlags is the NPCI control byte (byte 2 of the NPDU wire format).
type NPCIFieldsFlags uint8

// Flag masks for the NPCI byte per clause 6.2.2, Table 6-1 of ANSI/ASHRAE 135-2024.
const (
	networkPriorityMask       NPCIFieldsFlags = 0b00000011 // bits 1-0: network priority
	expectingReplyMask        NPCIFieldsFlags = 0b00000100 // bit 2: expecting reply
	sourceSpecifierMask       NPCIFieldsFlags = 0b00001000 // bit 3: source specifier present
	reservedBit4Mask          NPCIFieldsFlags = 0b00010000 // bit 4: reserved (must be 0)
	destinationSpecifierMask  NPCIFieldsFlags = 0b00100000 // bit 5: destination specifier present
	reservedBit6Mask          NPCIFieldsFlags = 0b01000000 // bit 6: reserved (must be 0)
	isNetworkLayerMessageMask NPCIFieldsFlags = 0b10000000 // bit 7: network layer message
)

type UltimateDestinationNetworkNumber uint16
type UltimateDestinationNetworkNumberMacAddressLength uint8
type UltimateDestinationMacLayerAddress []byte
type LocalNetworkDestinationMacLayerAddress []byte
type OriginalSourceNetworkNumber uint16
type OriginalSourceNetworkNumberMacAddressLength uint8
type OriginalSourceMacLayerAddress []byte
type LocalNetworkSourceMacLayerAddress []byte

func cloneBytes(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

// Package npdu provides BACnet network-layer NPDU modeling and wire encode/decode.
//
// It is intentionally APDU-agnostic: APDU payload bytes are carried as opaque
// data. Higher layers (for example, package apdu) are responsible for APDU
// semantics and use NPDU constructors to frame outbound traffic.
//
// For NPDU network-layer messages, package npdu also provides typed payload
// models and codecs via DecodeNetworkLayerMessageModel,
// NewNetworkLayerNPDUFromMessage, EncodeNetworkLayerMessageWire, and
// DecodeNetworkLayerMessageWire.
//
// Byte-slice ownership policy:
//   - NPDU constructors/decode defensively copy inbound slice data.
//   - Public accessors (for example APDUBytes, DADR, SADR) return defensive copies.
//   - Internal encode/decode paths avoid redundant re-cloning once ownership is clear.
package npdu

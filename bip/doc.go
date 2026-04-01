// Package bip provides a BACnet/IP (Annex J) BVLC starter scaffold.
//
// The package currently focuses on:
//   - BVLC type and function constants
//   - BVLC frame validation, decode, and encode helpers
//   - A minimal UDP datagram transport abstraction
//
// This is intentionally a small foundation for incremental NPDU/APDU
// integration while keeping the API lightweight and testable.
package bip

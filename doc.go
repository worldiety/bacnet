// Package bacnet provides foundational types and helpers for building BACnet/IP
// applications in pure Go.
//
// The package intentionally starts small: it defines common BACnet identifiers,
// protocol constants, validation helpers, and address primitives that are useful
// across client and server implementations. Encoding and transport layers such as
// BVLC, NPDU, and APDU handling can be added on top of this foundation.
//
// Scope
//   - Pure Go implementation with no cgo
//   - BACnet/IP focused
//   - Minimal dependencies (standard library only)
//   - Designed for incremental extension
//
// This library is intended to grow toward broader BACnet/IP support while keeping
// the initial API stable, lightweight, and easy to test.
package bacnet


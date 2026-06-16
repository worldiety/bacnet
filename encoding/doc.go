// Package encoding provides reusable BACnet tag/value codec helpers.
//
// The package centralizes low-level TLV parsing/encoding primitives that are
// shared by higher layers (for example apdu service payload codecs).
//
// Ownership rules:
//   - Inputs are never retained.
//   - Returned []byte values are newly allocated and owned by the caller.
package encoding

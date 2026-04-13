# bacnet

`bacnet` is a lightweight Go foundation for building BACnet/IP applications.
It currently provides root BACnet types and addressing primitives plus active
`bip`, `apdu`, and `npdu` packages for BVLC framing, APDU orchestration,
and NPDU encode/decode scaffolding.

## Goals

- Pure Go implementation
- No cgo
- Minimal dependencies (standard library only)
- Implementation of BACnet application and network layers as defined in ANSI/ASHRAE 135-2024
- BACnet implementation using IP in the link layer (BACnet/IP)
- Relying on the OS for physical layer and transport (UDP)
- Physical layer implementation is out of scope for this library
- Easy to test and extend

## Current foundation

The current implementation includes:

- Package documentation in `doc.go`
- BACnet/IP and identifier constants in `constants.go`
- Core BACnet types in `types.go`
- Validation and sentinel errors in `errors.go`
- Basic station/network addressing in `address.go`
- `bip/` for BACnet/IP and BACnet/IP6 BVLC frame encode/decode, all Annex J BVLC function structs, UDP datagram transport, BBMD handling, and IPv4 foreign-device support
- `apdu/` for interface-first ASE dispatch, confirmed invoke tracking, clause 5.4 state-machine scaffolding, and user-element wrappers
- `npdu/` for BACnet NPDU encode/decode, routed/local APDU constructors, and network-layer-message constructors
- `internal/util/` for shared defensive-copy helpers used across packages
- Unit tests across the implemented packages

## Project structure

```text
.
├── Agents.md
├── README.md
├── address.go
├── address_test.go
├── constants.go
├── doc.go
├── errors.go
├── errors_test.go
├── go.mod
├── types.go
├── types_test.go
├── apdu/        (active: BACnet application layer scaffold)
├── bip/         (active: BACnet/IP BVLC + transport + BBMD scaffold)
├── encoding/    (planned: BACnet tag/value encoding)
├── npdu/        (active: BACnet network layer scaffold)
├── lpdu/        (planned: BACnet IP link layer scaffold)
├── internal/    (active: non-public helpers; `internal/util` in active use)
├── testdata/    (planned: packet fixtures)
└── examples/    (deferred until API is stable)
```

The current implementation includes the root `bacnet` package together with active
`bip`, `apdu`, and `npdu` scaffolds. Planned directories remain extension points
for additional BACnet/IP layers.

The project is in a prototype phase: APIs are usable for experimentation and tests,
but may change as BACnet coverage expands.

## Example

```go
package main

import (
	"fmt"

	"go.wdy.de/bacnet"
)

func main() {
	deviceID, err := bacnet.NewObjectIdentifier(bacnet.ObjectTypeDevice, 1234)
	if err != nil {
		panic(err)
	}

	addr, err := bacnet.NewAddress(bacnet.LocalNetwork, []byte{192, 168, 1, 10, 0xBA, 0xC0})
	if err != nil {
		panic(err)
	}

	fmt.Println(deviceID)
	fmt.Println(addr.Network, addr.MACBytes())
}
```

## Run tests

```sh
go test ./...
go test -coverprofile=coverage.out ./...
```

## Next steps

Natural next additions for the library are:

1. Expanded BACnet tag/value encoding in `encoding/`
2. BACnet IP link-layer support in `lpdu/`
3. Expanded APDU wire compatibility for additional BACnet services
4. Higher-level BACnet/IP client workflows for discovery and property reads

## Notes

The module currently uses the local module path declared in `go.mod`:

```go
module go.wdy.de/bacnet
```

If you plan to publish the library, update that module path to your repository URL.

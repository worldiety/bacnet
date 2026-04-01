# bacnet

`bacnet` is a lightweight Go foundation for building BACnet/IP applications.
It currently provides core protocol constants, identifier types, validation
helpers, and addressing primitives that can be reused as the library grows
into fuller BACnet/IP support.

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

This scaffold includes:

- Package documentation in `doc.go`
- BACnet/IP and identifier constants in `constants.go`
- Core BACnet types in `types.go`
- Validation and sentinel errors in `errors.go`
- Basic station/network addressing in `address.go`
- Unit tests for the exported foundation

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
├── bip/         (active: BACnet/IP BVLC + transport scaffold)
├── encoding/    (planned: BACnet tag/value encoding)
├── npdu/        (planned: BACnet network layer)
├── internal/    (planned: non-public helpers)
├── testdata/    (planned: packet fixtures)
└── examples/    (deferred until API is stable)
```

The current implementation includes the root `bacnet` package plus active `bip` and
`apdu` scaffolds. Planned directories remain extension points for additional
BACnet/IP layers.

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
```

## Next steps

Natural next additions for the library are:

1. NPDU header parsing and serialization
2. Expanded APDU wire compatibility for additional services
3. Expanded BACnet/IP Annex J support (for example BBMD/FDT management)
4. A simple BACnet/IP client for discovery and property reads

## Notes

The module currently uses the local module path declared in `go.mod`:

```go
module go.wdy.de/bacnet
```

If you plan to publish the library, update that module path to your repository URL.

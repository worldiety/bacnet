# bacnet

`bacnet` is a lightweight Go foundation for building BACnet/IP applications.
It currently provides core protocol constants, identifier types, validation
helpers, and addressing primitives that can be reused as the library grows
into fuller BACnet/IP support.

## Goals

- Pure Go implementation
- No cgo
- Minimal dependencies (standard library only)
- BACnet/IP-first scope
- Easy to test and extend

## Current foundation

This scaffold includes:

- Package documentation in `doc.go`
- BACnet/IP and identifier constants in `constants.go`
- Core BACnet types in `types.go`
- Validation and sentinel errors in `errors.go`
- Basic station/network addressing in `address.go`
- Unit tests for the exported foundation

## Project layout

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
└── types.go
```

## Example

```go
package main

import (
	"fmt"

	"bacnet"
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

1. BACnet/IP BVLC frame encoding/decoding
2. NPDU header parsing and serialization
3. APDU support for core confirmed and unconfirmed services
4. A simple BACnet/IP client for discovery and property reads

## Notes

The module currently uses the local module path declared in `go.mod`:

```go
module bacnet
```

If you plan to publish the library, update that module path to your repository URL.


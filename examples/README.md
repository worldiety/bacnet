# Examples

The 'examples' directory contains example programs that demonstrate how to use the library. These examples are not 
meant to be exhaustive, but rather to illustrate how to set up the library and use its functionality.

A caveat of these examples is that they require a working BACnet device to interact with, so they are not included in the
test suite.

## Discover devices

The 'discoer' example shows how to use the library to discover BACnet devices on the local network.
It broadcasts a WhoIs request and listens for IAm responses from devices.

It can be run with:
```sh
go run ./examples/discover/main.go <Broadcast IP>
```

## Read property

```sh
go run examples/read_property/main.go <SensorIP>:<Port> <Broadcast IP>:<Port>
go run examples/read_property/main.go 192.168.8.194:47808 192.168.2.255:47808
```

## Write property

```sh
go run examples/write_property/main.go <SensorIP>:<Port> <Broadcast IP>:<Port>
go run examples/read_property/main.go 192.168.8.194:47808 192.168.2.255:47808
```

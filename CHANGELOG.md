## 0.2.0 (2026-06-17)

### Feat

- added error parsing to APDU

### Fix

- fixed wrong address semantics in netprim.Address

### Refactor

- refactored library structure: moved types, constants and functions from the bacnet package into a series of common packages, to prevent circular imports when creating a sensible public API in the bacnet package

## 0.1.0 (2026-06-16)

### Feat

- added encoding for missing BACnet types, moved some existing encoding functions into the ecoding package
- added logging to npdu
- added logging for errors
- added hop-count rejection, loop prevention, and handling of multiple routes per target
- integrated nlm encoding and validation into existing paths
- added wire encoding for mlm models
- added models for network layer messages (nlm) to npdu
- added npdu reject reasons
- added client runtime to enable creation of a bacnet-CLient
- moved apdu encoding to new encoding package
- added baseline encoding package
- added client Discover function
- added typed client methods for ReadRange, DeviceCommunicationCOntrol and ReinitializeDevice
- added typed client methods for ReadRange, DeviceCommunicationCOntrol and ReinitializeDevice
- added typed client COV and related functions
- added typed client object access
- added typed client for user to construct bacnet requests easily
- added type Stack to wrap Transport and connect bip to apdu
- added sending of unicast to bip.Device
- added translation between go net address and bacnet address
- added client unsegmented requests
- apdu: added out of order segment receive to server
- reworked apdu.ASE API to be more in line with the BACnet/OSI Layer model
- added transaction state machine scaffolding

### Fix

- fixed bug in router's loop suppression
- fixed inconsistencies in npdu validation
- fixed inconsistencies in network address and number validation across npdu and npdu/router
- segmented ACK acceptance was too strict, uses InWndow() now
- apdu changed types from primitives to specific types for most fields
- apdu server state machine: added duplicate ack for first segment
- fixed access to assembled payload not signaling when payload was not ready
- **apdu/codec.go**: fixed constant name
- **apdu/codec.go**: fixed wrong max apdu length

### Refactor

- changed errors when remote ends connection to contain more data
- setup basic project strcture

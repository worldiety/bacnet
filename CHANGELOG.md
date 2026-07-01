## 0.3.0 (2026-07-01)

### Feat

- added a general decoding function for data sent by a BACnet server
- added Default values for configurations

### Fix

- fixed encoding bug

## 0.2.0 (2026-06-17)

### Feat

- added error parsing to APDU
- added transaction state machine scaffolding

### Fix

- fixed wrong address semantics in netprim.Address

### Refactor

- refactored library structure: moved types, constants and functions from the bacnet package into a series of common packages, to prevent circular imports when creating a sensible public API in the bacnet package
- setup basic project strcture

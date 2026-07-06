## Unreleased

### Feat

- decode non-UTF-8 character strings: the character-string codec now handles the standard BACnet character sets (UTF-8, ISO-8859-1, UCS-2/UTF-16BE, UCS-4/UTF-32BE) and yields a proper AppCharacterString, so client.PropertyValue Text() and Display() work without a recovery path (e.g. Kieback&Peter "Außentemperatur")
- added client.PropertyValue.Charset() to inspect the on-the-wire character set of a character string

### Fix

- character-string decoding no longer fails on non-conformant devices: unknown or malformed encodings fall back to a readable ISO-8859-1 interpretation instead of returning an error

### Refactor

- replaced encoding.EncodeCharacterStringASCIIValue / DecodeCharacterStringASCIIValue with charset-aware encoding.EncodeCharacterStringValue / DecodeCharacterStringValue (breaking: the *ASCIIValue functions were removed)

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

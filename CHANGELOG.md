## Unreleased

### Feat

- decode non-UTF-8 character strings: the character-string codec now handles the standard BACnet character sets (UTF-8, ISO-8859-1, UCS-2/UTF-16BE, UCS-4/UTF-32BE) and yields a proper AppCharacterString, so client.PropertyValue Text() and Display() work without a recovery path (e.g. Kieback&Peter "Außentemperatur")
- added client.PropertyValue.Charset() to inspect the on-the-wire character set of a character string
- added high-level ReadPropertyMultiple support to the client package: client.ReadPropertiesMultiple (one object) and client.ReadMultiple (many objects) read in a single request and fall back automatically to per-property reads when a device lacks RPM, when the response would not fit one APDU, or when the device rejects the whole request because a requested property/object is not applicable (e.g. an inapplicable network-port property) — the fallback isolates the bad property so the valid ones still return values
- added high-level WritePropertyMultiple support: client.WritePropertiesMultiple writes several properties/objects as one atomic request (no automatic fallback, to preserve write semantics)
- client.PropertyValue now decodes and renders list-valued properties (e.g. object-list, state-text, property-list): added Values, Len(), IsList() and ObjectIDs(), and Display() renders a list as "[a, b, c]"
- the bacnetf `props` command now uses ReadPropertyMultiple by default (with a --no-rpm opt-out)
- added apdu.ReadPropertyResult.DecodeError() to decode a per-property RPM error into its error-class and error-code
- discover devices behind a BACnet router: Discover/Resolve now send Who-Is as a global broadcast (DNET 0xFFFF) by default so routers forward it to remote networks (e.g. MS/TP segments); use client.WithLocalOnly (bacnetf discover --local-only) for a local-only Who-Is
- address devices on remote networks: netprim.Address carries the originating network number and MAC (SNET/SADR) decoded from routed replies, and requests to such devices are sent to the router with the correct NPDU destination (DNET/DADR). client.Device gains MAC, IsRouted() and Target(); bacnetf discover shows the network and MAC for routed devices
- added netprim.NewRoutedAddress and netprim.Address.IsRouted/String for remote-network (routed) stations

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

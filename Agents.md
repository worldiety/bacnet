## Project description
This project is a go implementation of the Bacnet standard. It is designed to be a lightweight and efficient library for building Bacnet applications in Go.
The project focuses on the IP protocol, other Bacnet protocols may be implemented in the future, but are NOT the current scope of this project.

## Technical requirements
- The project must be implemented in Go.
- The project must be well-documented and easy to use.
- The project must be efficient and lightweight, with minimal dependencies.
- The project must NOT use cgo. Ever. Make sure dependencies do not use cgo. If you need to use a dependency that uses cgo, find an alternative that does not use cgo, or notify the user.

## Dependency requirements
- Do not use dependencies unless absolutely necessary. If you need to use a dependency, make sure it is well-maintained and has a good reputation. Double check with the user whether using a dependency is okay.

## Development requirements
- The project must be developed using best practices for Go development, including proper error handling.
- The project must include unit tests for all major functionality, with a goal of achieving at least 80% code coverage.
- The example directory is ignored for development until the project has a stable API (and this line is removed)

## Bacnet standard requirements
- The project must implement the Bacnet standard as defined in the official Bacnet documentation.


# Iznik Server Go

Iznik is a platform for online reuse of unwanted items.  This is the fast API server, written in Go.

There is a Docker Compose development environment which can be used to run a complete standalone system; see [FreegleDocker](https://github.com/Freegle/FreegleDocker).

## What this is for
The aim is to provide fast read-only access, so that we can:
* Render pages significantly faster than when using the [PHP server](https://github.com/Freegle/iznik-server).  Language wars are dull, but Go is faster, and the easy parallelisation which goroutines offer make it possible to reduce the latency of individual calls dramatically.
* Reduce the CPU load on our limited server hardware.  Most Freegle workload is read operations, and Go is much lighter on CPU than PHP.

So although having two servers in different languages is an niffy architecture smell, the nifty practical benefits are huge.

## What this is not for

These are out of scope:
* Access to data which is private to moderators.
* Almost all write-access or any kind of actions.

Those things are done using the PHP API.  

## Testing

**Note:** Go tests for this repository now run as part of the [FreegleDocker](https://github.com/Freegle/FreegleDocker) CircleCI pipeline for integration testing. This ensures tests run against the complete Docker Compose environment with all services available.

The CircleCI configuration in this repository has been updated to skip local tests and redirect to the FreegleDocker pipeline. Coverage reporting still uploads to Coveralls from the FreegleDocker environment.

## API Documentation

The API is documented using Swagger/OpenAPI. To view the API documentation:

1. Set up the go-swagger tool:

   **Option 1: Automatic download (recommended)**

   The scripts will automatically try to download and use a local copy of go-swagger if you don't have it installed:

   ```
   # Just run the generate script and it will download swagger if needed
   ./generate-swagger.sh
   ```

   **Option 2: Manual installation**

   Alternatively, you can install go-swagger globally:

   ```
   go install github.com/go-swagger/go-swagger/cmd/swagger@v0.30.5
   ```

   Note: We use a specific version (v0.30.5) to avoid compatibility issues with newer versions that may use Go directives not supported by all Go versions.

2. Generate the Swagger documentation:

   **Manual generation:**

   On Linux/macOS:
   ```
   ./generate-swagger.sh
   ```

   On Windows:
   ```
   generate-swagger.bat
   ```

   Or if you have Git Bash or WSL on Windows:
   ```
   ./generate-swagger.sh
   ```

   **Automatic generation during build:**

   The Swagger documentation is automatically generated when building the application using:

   On Linux/macOS:
   ```
   ./build.sh
   ```

   On Windows:
   ```
   build.bat
   ```

3. Start the server and access the Swagger UI at:
   ```
   http://localhost:8192/swagger/
   ```

The Swagger documentation provides a complete reference of all API endpoints, parameters, and response formats.

### Documentation Notes

- The Swagger documentation is generated in two main ways:
  1. Model definitions: From annotated types in your code (e.g., `message.go`)
  2. API paths: From route definitions in `swagger/swagger.go`

- If you add new routes to the API:
  1. Add a new route definition to `swagger/swagger.go` using the `swagger:route` annotation
  2. Define response types in `swagger/swagger.go` using the `swagger:response` annotation
  3. Make sure each path parameter has a unique example ID
  4. Add any parameters (path, query, body) with proper descriptions
  5. Run the generate script to ensure your routes appear in the documentation

The API documentation includes:
- Full endpoint descriptions
- Request parameters with examples
- Response formats and status codes
- Security requirements

## Funding
The development has been funded by Freegle for use in the UK,
but it is an open source platform which can be used or adapted by others.

**It would be very lovely if you sponsored us.**

[:heart: Sponsor](https://github.com/sponsors/Freegle)

## License

This code is licensed under the GPL v2 (see LICENSE file).  If you intend to use it, Freegle would be interested to
hear about it; you can mail [geeks@ilovefreegle.org](mailto:geeks@ilovefreegle.org).

[![CircleCI](https://dl.circleci.com/status-badge/img/gh/Freegle/iznik-server-go/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/Freegle/iznik-server-go/tree/master)

[![Coverage Status](https://coveralls.io/repos/github/Freegle/iznik-server-go/badge.svg)](https://coveralls.io/github/Freegle/iznik-server-go)
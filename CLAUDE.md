# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Iznik is a platform for online reuse of unwanted items. This is the fast API server written in Go that provides read-only access to complement the main PHP server. The Go server is optimized for performance and handles most read operations, while write operations are typically handled by the PHP API.

## Common Commands

### Building the Application
```bash
# Build the application (includes Swagger generation)
./build.sh

# On Windows
build.bat
```

### Running Tests
```bash
# Run all tests
go test ./test/...

# Run specific test file
go test ./test/message_test.go

# Run tests with verbose output
go test -v ./test/...
```

### Development Server
```bash
# Start the development server (default port 8192)
go run main.go

# Server will be available at http://localhost:8192
# Swagger documentation at http://localhost:8192/swagger/
```

### Swagger Documentation
```bash
# Generate Swagger documentation only
./generate-swagger.sh

# On Windows
generate-swagger.bat
```

## Architecture Overview

### Core Components

**Main Application (`main.go`)**
- Entry point for both standalone server and AWS Lambda deployment
- Configures Fiber web framework with compression, CORS, and error handling
- Sets up database connection and middleware
- Serves on port 8192 in standalone mode

**Database Layer (`database/`)**
- Uses GORM for ORM with MySQL driver
- Includes connection pooling and query cancellation support
- Database configuration via environment variables
- Separate ping middleware for health checks

**Authentication (`user/authMiddleware.go`)**
- JWT-based authentication middleware
- Validates user sessions against database
- Integrates with Sentry for error tracking
- Runs asynchronously for performance

**API Routes (`router/routes.go`)**
- Comprehensive REST API with both `/api` and `/apiv2` endpoints
- Swagger/OpenAPI documentation annotations
- Routes organized by domain (messages, users, groups, etc.)

### Domain Structure

The application follows a domain-driven structure with each domain in its own package:

- **`address/`** - Address and location data
- **`chat/`** - Chat rooms and messaging
- **`communityevent/`** - Community events and dates
- **`group/`** - User groups and group management
- **`message/`** - Core messaging functionality (offers, wants, etc.)
- **`user/`** - User accounts and authentication
- **`volunteering/`** - Volunteering opportunities
- **`story/`** - User stories and testimonials
- **`newsfeed/`** - News feed items
- **`notification/`** - User notifications

### Key Technical Details

**Environment Variables Required:**
- `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_DBNAME`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_PROTOCOL`
- `FUNCTIONS` (set for AWS Lambda deployment)
- `USER_SITE` (affects compression settings)

**Performance Optimizations:**
- Goroutine-based concurrent processing
- Connection pooling for database
- Compression middleware (disabled for local development)
- JWT validation runs asynchronously

**Testing:**
- Comprehensive test suite in `test/` directory
- Uses `testUtils.go` for common test utilities
- Tests cover all major domains and API endpoints
- Shared Fiber app instance for testing (`test/main_test.go`)

## Swagger Documentation

The API uses Swagger annotations for documentation generation:

- Route definitions in `router/routes.go` use `@Router` annotations
- Model definitions use struct tags for JSON serialization
- Additional route documentation in `swagger/swagger.go`
- Generated documentation available at `/swagger/` endpoint

When adding new API endpoints:
1. Add route annotations in `router/routes.go`
2. Define response types in `swagger/swagger.go` if needed
3. Run `./generate-swagger.sh` to regenerate documentation
4. Ensure unique example IDs for path parameters

## Development Notes

- Server runs in UTC timezone (set in `main.go`)
- Uses Fiber v2 web framework with custom error handling
- Database queries use raw SQL for performance in some cases
- Supports both standalone and AWS Lambda deployment modes
- CORS is enabled for all origins with 24-hour preflight caching

## Testing Considerations

- Never run tests when in a WSL environment.
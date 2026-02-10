# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

New API is a next-generation AI model gateway and asset management system built in Go with a React frontend. It's based on One API and provides unified access to multiple AI providers (OpenAI, Claude, Gemini, etc.) with enhanced features like user management, billing, caching, and conversation history.

## Development Commands

### Backend Development
```bash
# Run backend development server
go run main.go

# Build the backend binary
go build -o one-api main.go

# Run tests (requires database setup)
go test ./test/...
```

### Frontend Development
```bash
# Install dependencies (uses Bun package manager)
cd web && bun install

# Start frontend development server
cd web && bun run dev

# Build frontend for production
cd web && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

# Lint frontend code
cd web && bun run lint
cd web && bun run eslint
```

### Full Stack Development
```bash
# Build frontend and start backend (using Makefile)
make all

# Build frontend only
make build-frontend

# Start backend only
make start-backend
```

### Docker Development
```bash
# Run with Docker Compose (recommended)
docker-compose up -d

# Build and run with Docker
docker build -t new-api .
docker run -p 3000:3000 -v ./data:/data new-api
```

## Architecture Overview

### Core Components

**Backend (Go)**
- `main.go` - Application entry point with initialization and HTTP server setup
- `model/` - Database models and ORM logic (GORM-based)
- `controller/` - HTTP request handlers and business logic
- `service/` - Business services and external API integrations
- `middleware/` - HTTP middleware (auth, logging, rate limiting, etc.)
- `router/` - HTTP route definitions
- `common/` - Shared utilities, constants, and helper functions
- `dto/` - Data transfer objects for API requests/responses
- `relay/` - AI provider relay and request forwarding logic

**Frontend (React + Semi UI)**
- Built with Vite, uses Semi Design UI components
- Located in `web/` directory
- Embedded into Go binary at build time via `embed.FS`

### Database Architecture

**Multi-Database Setup:**
- **Main DB**: Core application data (users, channels, tokens, etc.)
- **Log DB**: Request/response logs and usage tracking
- **MES DB**: Message/conversation history storage with optional daily partitioning

**Database Support:**
- SQLite (default, suitable for single instance)
- MySQL (5.7.8+)
- PostgreSQL (9.6+)

**Connection Configuration:**
- `SQL_DSN` - Main database connection string
- `LOG_SQL_DSN` - Log database connection string
- `MES_SQL_DSN` - MES database connection string
- `SQLITE_PATH` - SQLite database file path

### Channel System

The core abstraction for AI provider integration:
- **Channel**: Represents an AI provider endpoint (OpenAI, Claude, etc.)
- **Channel Types**: Different provider implementations in `relay/` directory
- **Load Balancing**: Weighted random distribution across channels
- **Health Monitoring**: Automatic channel testing and status updates
- **Caching**: Redis-based channel metadata caching

### Key Features

**Request Format Conversion:**
- OpenAI Chat Completions ⇔ Claude Messages
- OpenAI Chat Completions ⇔ Gemini Chat
- Supports Claude Code integration via format conversion

**Billing & Usage:**
- Token-based billing with configurable pricing
- Usage quotas and rate limiting
- Multiple payment methods (Stripe, 易支付)
- Cache-aware billing with configurable cache hit rates

**User Management:**
- JWT-based authentication with multiple OAuth providers
- User groups and permissions
- Token management with model restrictions

## Environment Variables

### Essential Configuration
```bash
PORT=3000                    # Server port
SESSION_SECRET=random_string # Session encryption key
```

### Database Configuration
```bash
SQL_DSN=user:pass@tcp(host:port)/db           # Main database
LOG_SQL_DSN=user:pass@tcp(host:port)/logdb    # Log database
MES_SQL_DSN=user:pass@tcp(host:port)/mesdb    # MES database
SQLITE_PATH=/data/one-api.db                  # SQLite path
```

### Cache Configuration
```bash
REDIS_CONN_STRING=redis://user:pass@host:port/0  # Redis connection
MEMORY_CACHE_ENABLED=true                        # Enable memory cache
SYNC_FREQUENCY=60                                 # Cache sync interval (seconds)
```

### Feature Toggles
```bash
UPDATE_TASK=true                    # Enable async task updates (Midjourney, Suno)
GENERATE_DEFAULT_TOKEN=false        # Generate tokens for new users
DIFY_DEBUG=true                     # Debug Dify workflow outputs
BATCH_UPDATE_ENABLED=true           # Enable batch database updates
```

### Multi-Site Deployment
```bash
SITE_ID=default                     # Site identifier for session isolation
CRYPTO_SECRET=encryption_key        # Encryption key for sensitive data
```

## File Organization

### Backend Structure
- Configuration and constants: `constant/`, `setting/`
- Business logic: `controller/`, `service/`
- Data layer: `model/`, database models and helpers
- HTTP layer: `router/`, `middleware/`
- Provider integrations: `relay/` (organized by provider)
- Shared utilities: `common/`, `types/`

### Frontend Structure
- React application in `web/`
- Semi Design UI components
- Built assets embedded in Go binary
- Proxy configuration for development in `web/package.json`

### Testing Structure
- Go tests in `test/` directory
- Example usage patterns in `test/mes_test_example.go`
- Database setup required for integration tests

## Development Patterns

### Database Models
- Uses GORM ORM with struct tags for table mapping
- Soft deletes enabled for most models
- Migration handling in `model/` initialization
- Batch operations available for performance

### Error Handling
- Structured error responses in `dto/error.go`
- Error logging with configurable levels
- Custom error types for different scenarios

### API Providers
- New providers added in `relay/` directory
- Provider-specific request/response handling
- Common interface for unified access
- Health check implementations required

### Caching Strategy
- Redis for distributed caching (multi-instance deployments)
- In-memory caching for single instances
- Channel metadata caching for performance
- Configurable cache TTL and sync intervals

### Security Considerations
- JWT token validation in middleware
- API key authentication for channels
- Rate limiting per user/endpoint
- Request/response sanitization
- No sensitive data in logs (tokens are masked)

## Common Issues

### Database Connection
- Ensure proper charset and timezone settings for MySQL
- Use `parseTime=true` in MySQL DSN for datetime handling
- PostgreSQL requires SSL configuration in production

### Multi-Instance Deployment
- Must set `SESSION_SECRET` for consistent sessions
- Must set `CRYPTO_SECRET` for shared Redis data
- Consider database connection pooling limits

### Performance Optimization
- Enable Redis caching for channel metadata
- Use batch updates for high-volume operations
- Configure appropriate connection pool sizes
- Monitor and tune `SYNC_FREQUENCY` setting

### Provider Integration
- Each provider may have specific authentication requirements
- Rate limits vary by provider and plan
- Some providers require special headers or request formats
- Test channels regularly with automated health checks

## Testing and Quality Assurance

### Running Tests
```bash
# Run all Go tests (requires database setup)
go test ./test/...

# Run a specific test file
go test ./test/mes_test_example.go -v

# Run tests with coverage
go test ./test/... -cover
```

### Code Quality
```bash
# Frontend linting and formatting
cd web && bun run lint        # Check formatting with Prettier
cd web && bun run lint:fix    # Auto-fix formatting issues
cd web && bun run eslint      # Check code quality with ESLint
cd web && bun run eslint:fix  # Auto-fix ESLint issues

# Go formatting and vetting
go fmt ./...                  # Format Go code
go vet ./...                  # Check for Go code issues
```

## Important Development Notes

### Database Initialization
- The application uses GORM for database operations with auto-migration
- Three separate databases are supported: main, log, and MES (messages)
- SQLite is used by default; MySQL 5.7.8+ and PostgreSQL 9.6+ are also supported
- For testing, ensure proper database configuration is set up

### Frontend Build Integration
- Frontend assets are embedded into the Go binary using `embed.FS`
- The build process uses Vite with React and Semi Design UI components
- Frontend proxy is configured to route API calls to `http://localhost:3000` during development
- Environment variable `VITE_REACT_APP_VERSION` is set from the VERSION file during build

### AI Provider Integration Architecture
- The `relay/` directory contains provider-specific implementations
- Each provider must implement a common interface for unified access
- Channel system provides load balancing, health monitoring, and caching
- Request/response format conversion is handled automatically (OpenAI ↔ Claude ↔ Gemini)

### Configuration Management
- Uses environment variables for all configuration
- Supports `.env` file loading via `godotenv`
- Multi-site deployment requires `SESSION_SECRET` and `CRYPTO_SECRET` for consistency
- Redis is used for distributed caching in multi-instance deployments# CLAUDE.md — Project Conventions for new-api

## Overview

This is an AI API gateway/proxy built with Go. It aggregates 40+ upstream AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, etc.) behind a unified API, with user management, billing, rate limiting, and an admin dashboard.

## Tech Stack

- **Backend**: Go 1.22+, Gin web framework, GORM v2 ORM
- **Frontend**: React 18, Vite, Semi Design UI (@douyinfe/semi-ui)
- **Databases**: SQLite, MySQL, PostgreSQL (all three must be supported)
- **Cache**: Redis (go-redis) + in-memory cache
- **Auth**: JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, OIDC, etc.)
- **Frontend package manager**: Bun (preferred over npm/yarn/pnpm)

## Architecture

Layered architecture: Router -> Controller -> Service -> Model

```
router/        — HTTP routing (API, relay, dashboard, web)
controller/    — Request handlers
service/       — Business logic
model/         — Data models and DB access (GORM)
relay/         — AI API relay/proxy with provider adapters
  relay/channel/ — Provider-specific adapters (openai/, claude/, gemini/, aws/, etc.)
middleware/    — Auth, rate limiting, CORS, logging, distribution
setting/       — Configuration management (ratio, model, operation, system, performance)
common/        — Shared utilities (JSON, crypto, Redis, env, rate-limit, etc.)
dto/           — Data transfer objects (request/response structs)
constant/      — Constants (API types, channel types, context keys)
types/         — Type definitions (relay formats, file sources, errors)
i18n/          — Backend internationalization (go-i18n, en/zh)
oauth/         — OAuth provider implementations
pkg/           — Internal packages (cachex, ionet)
web/           — React frontend
  web/src/i18n/  — Frontend internationalization (i18next, zh/en/fr/ru/ja/vi)
```

## Internationalization (i18n)

### Backend (`i18n/`)
- Library: `nicksnyder/go-i18n/v2`
- Languages: en, zh

### Frontend (`web/src/i18n/`)
- Library: `i18next` + `react-i18next` + `i18next-browser-languagedetector`
- Languages: zh (fallback), en, fr, ru, ja, vi
- Translation files: `web/src/i18n/locales/{lang}.json` — flat JSON, keys are Chinese source strings
- Usage: `useTranslation()` hook, call `t('中文key')` in components
- Semi UI locale synced via `SemiLocaleWrapper`
- CLI tools: `bun run i18n:extract`, `bun run i18n:sync`, `bun run i18n:lint`

## Rules

### Rule 1: JSON Package — Use `common/json.go`

All JSON marshal/unmarshal operations MUST use the wrapper functions in `common/json.go`:

- `common.Marshal(v any) ([]byte, error)`
- `common.Unmarshal(data []byte, v any) error`
- `common.UnmarshalJsonStr(data string, v any) error`
- `common.DecodeJson(reader io.Reader, v any) error`
- `common.GetJsonType(data json.RawMessage) string`

Do NOT directly import or call `encoding/json` in business code. These wrappers exist for consistency and future extensibility (e.g., swapping to a faster JSON library).

Note: `json.RawMessage`, `json.Number`, and other type definitions from `encoding/json` may still be referenced as types, but actual marshal/unmarshal calls must go through `common.*`.

### Rule 2: Database Compatibility — SQLite, MySQL >= 5.7.8, PostgreSQL >= 9.6

All database code MUST be fully compatible with all three databases simultaneously.

**Use GORM abstractions:**
- Prefer GORM methods (`Create`, `Find`, `Where`, `Updates`, etc.) over raw SQL.
- Let GORM handle primary key generation — do not use `AUTO_INCREMENT` or `SERIAL` directly.

**When raw SQL is unavoidable:**
- Column quoting differs: PostgreSQL uses `"column"`, MySQL/SQLite uses `` `column` ``.
- Use `commonGroupCol`, `commonKeyCol` variables from `model/main.go` for reserved-word columns like `group` and `key`.
- Boolean values differ: PostgreSQL uses `true`/`false`, MySQL/SQLite uses `1`/`0`. Use `commonTrueVal`/`commonFalseVal`.
- Use `common.UsingPostgreSQL`, `common.UsingSQLite`, `common.UsingMySQL` flags to branch DB-specific logic.

**Forbidden without cross-DB fallback:**
- MySQL-only functions (e.g., `GROUP_CONCAT` without PostgreSQL `STRING_AGG` equivalent)
- PostgreSQL-only operators (e.g., `@>`, `?`, `JSONB` operators)
- `ALTER COLUMN` in SQLite (unsupported — use column-add workaround)
- Database-specific column types without fallback — use `TEXT` instead of `JSONB` for JSON storage

**Migrations:**
- Ensure all migrations work on all three databases.
- For SQLite, use `ALTER TABLE ... ADD COLUMN` instead of `ALTER COLUMN` (see `model/main.go` for patterns).

### Rule 3: Frontend — Prefer Bun

Use `bun` as the preferred package manager and script runner for the frontend (`web/` directory):
- `bun install` for dependency installation
- `bun run dev` for development server
- `bun run build` for production build
- `bun run i18n:*` for i18n tooling

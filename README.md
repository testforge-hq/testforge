# TestForge

AI-powered test automation platform that automatically discovers, designs, generates, and executes end-to-end tests for web applications.

## Features

- **Intelligent Discovery**: Playwright-based crawler that automatically discovers pages, forms, and user flows
- **AI Test Design**: Uses Claude AI to generate comprehensive test cases with BDD format
- **Script Generation**: Produces production-ready Playwright TypeScript test scripts
- **Self-Healing Tests**: AI-powered automatic repair of broken tests
- **Visual Testing**: VJEPA-powered visual regression detection
- **Multi-tenant**: Full multi-tenant support with project isolation
- **Workflow Orchestration**: Temporal-based reliable workflow execution

## Architecture

```
                                    +-------------------+
                                    |   Web Dashboard   |
                                    +--------+----------+
                                             |
                                    +--------v----------+
                                    |    API Server     |
                                    |   (Go + Chi)      |
                                    +--------+----------+
                                             |
              +------------------------------+------------------------------+
              |                              |                              |
    +---------v---------+         +----------v----------+         +--------v--------+
    |   PostgreSQL      |         |      Temporal       |         |     Redis       |
    |   (Data Store)    |         |   (Orchestration)   |         |    (Cache)      |
    +-------------------+         +----------+----------+         +-----------------+
                                             |
              +------------------------------+------------------------------+
              |                              |                              |
    +---------v---------+         +----------v----------+         +--------v--------+
    |    Discovery      |         |    Test Design      |         |   Script Gen    |
    |    (Playwright)   |         |    (Claude AI)      |         |  (Playwright)   |
    +-------------------+         +---------------------+         +-----------------+
              |                              |                              |
              +------------------------------+------------------------------+
                                             |
              +------------------------------+------------------------------+
              |                              |                              |
    +---------v---------+         +----------v----------+         +--------v--------+
    |    Execution      |         |    Self-Healing     |         |    Reporting    |
    |    (Playwright)   |         |    (Claude AI)      |         |   (Claude AI)   |
    +-------------------+         +---------------------+         +-----------------+
```

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js 18+ (for Playwright)
- PostgreSQL 14+
- Redis 7+
- Temporal Server
- Anthropic API Key

### Installation

```bash
# Clone the repository
git clone https://github.com/testforge/testforge.git
cd testforge

# Install Go dependencies
go mod download

# Install Playwright browsers
npx playwright install chromium

# Set up environment variables
cp .env.example .env
# Edit .env with your configuration
```

### Running the Demo

The quickest way to see TestForge in action:

```bash
# Set your Anthropic API key
export ANTHROPIC_API_KEY=your-api-key

# Run the demo
go run cmd/demo/main.go --url="https://example.com" --max-pages=5
```

### Running the API Server

```bash
# Start dependencies (PostgreSQL, Redis, Temporal)
docker-compose up -d

# Run database migrations
go run cmd/migrate/main.go up

# Start the API server
go run cmd/api/main.go
```

## Project Structure

```
testforge/
├── cmd/                    # Application entry points
│   ├── api/               # REST API server
│   ├── demo/              # Standalone demo
│   ├── worker/            # Temporal worker
│   ├── testdesign/        # Test design CLI
│   └── scriptgen/         # Script generation CLI
├── internal/
│   ├── api/               # HTTP handlers and routes
│   ├── config/            # Configuration management
│   ├── domain/            # Domain models and interfaces
│   ├── llm/               # LLM client (Claude)
│   ├── repository/        # Data access layer
│   └── services/          # Business logic
│       ├── discovery/     # Web crawler
│       ├── testdesign/    # AI test design
│       ├── scriptgen/     # Script generation
│       ├── execution/     # Test execution
│       ├── healing/       # Self-healing
│       └── reporting/     # Report generation
├── pkg/                   # Shared utilities
├── services/              # Microservices
│   └── visual-ai/         # Visual testing service (Python)
├── docs/                  # Documentation
│   ├── api/              # API documentation
│   └── architecture/     # Architecture docs
└── migrations/            # Database migrations
```

## API Documentation

Full API documentation is available:

- **OpenAPI Spec**: `docs/api/openapi.yaml`
- **Swagger UI**: http://localhost:8080/docs (when running API server)

### Quick API Examples

```bash
# Create a tenant
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "My Company", "slug": "my-company"}'

# Create a project
curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/projects \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -d '{"name": "E-Commerce Site", "base_url": "https://example.com"}'

# Start a test run
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -d '{"project_id": "$PROJECT_ID"}'

# Get test run status
curl http://localhost:8080/api/v1/runs/$RUN_ID \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT_ID"
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key for Claude | Required |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://localhost/testforge` |
| `REDIS_URL` | Redis connection string | `redis://localhost:6379` |
| `TEMPORAL_HOST` | Temporal server address | `localhost:7233` |
| `PORT` | API server port | `8080` |
| `LOG_LEVEL` | Logging level | `info` |

### Configuration File

Create `config.yaml` for detailed configuration:

```yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

database:
  host: localhost
  port: 5432
  name: testforge
  user: postgres
  password: secret
  max_open_conns: 25
  max_idle_conns: 5

redis:
  host: localhost
  port: 6379
  db: 0

temporal:
  host: localhost
  port: 7233
  namespace: testforge

llm:
  model: claude-sonnet-4-20250514
  max_tokens: 16384
  temperature: 0.7
  rate_limit_rpm: 50
```

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/services/discovery/...
```

### Building

```bash
# Build all binaries
go build -o bin/ ./cmd/...

# Build specific binary
go build -o bin/api ./cmd/api
```

### Linting

```bash
# Run golangci-lint
golangci-lint run

# Fix auto-fixable issues
golangci-lint run --fix
```

## Test Run Workflow

When you start a test run, TestForge executes the following stages:

1. **Discovery** - Crawls the target URL using Playwright
   - Discovers pages, forms, buttons, links
   - Identifies user flows and business processes
   - Captures screenshots for visual testing

2. **Test Design** - AI generates test cases
   - Analyzes discovered pages and flows
   - Creates comprehensive test cases in BDD format
   - Prioritizes by risk and importance

3. **Script Generation** - Creates Playwright scripts
   - Generates TypeScript test files
   - Creates Page Object Models
   - Includes fixtures and utilities

4. **Execution** - Runs the generated tests
   - Parallel test execution
   - Captures screenshots, videos, traces
   - Collects execution metrics

5. **Self-Healing** - Repairs broken tests
   - Analyzes test failures
   - Uses AI to find alternative selectors
   - Updates tests automatically

6. **Reporting** - Generates comprehensive reports
   - Execution summary with pass/fail rates
   - AI-generated insights and recommendations
   - Visual comparison results

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- Documentation: https://docs.testforge.io
- Issues: https://github.com/testforge/testforge/issues
- Email: support@testforge.io

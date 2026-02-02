# Getting Started with TestForge

This guide will help you set up and run your first automated test with TestForge.

## Prerequisites

Before you begin, ensure you have the following installed:

- **Go 1.21+**: [Download Go](https://golang.org/dl/)
- **Node.js 18+**: [Download Node.js](https://nodejs.org/)
- **PostgreSQL 14+**: [Download PostgreSQL](https://www.postgresql.org/download/)
- **Redis 7+**: [Download Redis](https://redis.io/download/)
- **Docker** (optional): For running dependencies

You'll also need an **Anthropic API Key** for Claude AI integration.

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/testforge/testforge.git
cd testforge
```

### 2. Install Go Dependencies

```bash
go mod download
```

### 3. Install Playwright

```bash
npx playwright install chromium
```

### 4. Set Up Environment

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```env
# Required
ANTHROPIC_API_KEY=sk-ant-...

# Database
DATABASE_URL=postgres://postgres:password@localhost:5432/testforge?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379

# API Server
PORT=8080
LOG_LEVEL=info
```

### 5. Start Dependencies

Using Docker Compose (recommended):

```bash
docker-compose up -d
```

Or start services manually:

```bash
# PostgreSQL
pg_ctl -D /usr/local/var/postgres start

# Redis
redis-server

# Temporal (if using workflows)
temporal server start-dev
```

### 6. Run Database Migrations

```bash
go run cmd/migrate/main.go up
```

## Quick Start: Running the Demo

The fastest way to see TestForge in action is using the demo command:

```bash
# Set your API key
export ANTHROPIC_API_KEY=sk-ant-...

# Run against a sample website
go run cmd/demo/main.go --url="https://automationexercise.com" --max-pages=5
```

This will:
1. Discover pages on the target website
2. Generate test cases using AI
3. Create Playwright test scripts
4. Execute the tests
5. Generate a report

## Using the API

### Start the API Server

```bash
go run cmd/api/main.go
```

The server starts at `http://localhost:8080`.

### Create a Tenant

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Company",
    "slug": "my-company"
  }'
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My Company",
  "slug": "my-company",
  "plan": "free",
  "settings": {
    "max_concurrent_runs": 5,
    "enable_self_healing": true
  },
  "created_at": "2024-01-15T10:00:00Z"
}
```

### Create a Project

```bash
curl -X POST http://localhost:8080/api/v1/tenants/550e8400-e29b-41d4-a716-446655440000/projects \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{
    "name": "E-Commerce Tests",
    "base_url": "https://automationexercise.com",
    "settings": {
      "default_browser": "chromium",
      "capture_screenshots": true,
      "capture_video": true
    }
  }'
```

### Start a Test Run

```bash
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{
    "project_id": "660e8400-e29b-41d4-a716-446655440001"
  }'
```

### Monitor Test Run Status

```bash
curl http://localhost:8080/api/v1/runs/770e8400-e29b-41d4-a716-446655440002 \
  -H "X-Tenant-ID: 550e8400-e29b-41d4-a716-446655440000"
```

Status values:
- `pending` - Waiting to start
- `discovering` - Crawling the website
- `designing` - Generating test cases
- `automating` - Creating test scripts
- `executing` - Running tests
- `healing` - Repairing broken tests
- `reporting` - Generating report
- `completed` - Finished successfully
- `failed` - Encountered an error
- `cancelled` - Manually cancelled

## Using the CLI Tools

### Discovery Only

Crawl a website without generating tests:

```bash
go run cmd/discovery/main.go \
  --url="https://example.com" \
  --max-pages=10 \
  --output=discovery.json
```

### Test Design Only

Generate tests from a discovery result:

```bash
go run cmd/testdesign/main.go \
  --app-model=discovery.json \
  --output=test_suite.json
```

### Script Generation Only

Generate Playwright scripts from a test suite:

```bash
go run cmd/scriptgen/main.go \
  --suite=test_suite.json \
  --output=./generated
```

### Run Generated Tests

```bash
cd generated
npm install
npx playwright install
npm test
```

## Configuration Options

### Discovery Options

| Flag | Description | Default |
|------|-------------|---------|
| `--url` | Target URL to crawl | Required |
| `--max-pages` | Maximum pages to discover | 10 |
| `--max-depth` | Maximum crawl depth | 3 |
| `--timeout` | Page load timeout | 30s |
| `--headless` | Run browser headless | true |
| `--screenshot-dir` | Directory for screenshots | ./screenshots |

### Test Design Options

| Flag | Description | Default |
|------|-------------|---------|
| `--app-model` | Path to discovery JSON | Required |
| `--url` | Run discovery first | - |
| `--output` | Output file path | - |
| `--security` | Include security tests | true |
| `--accessibility` | Include a11y tests | true |

### Script Generation Options

| Flag | Description | Default |
|------|-------------|---------|
| `--suite` | Path to test suite JSON | Required |
| `--output` | Output directory | ./generated |
| `--base-url` | Override base URL | - |
| `--typescript` | Use TypeScript strict mode | true |

## Viewing Reports

After a test run completes, you can view the report:

1. **HTML Report**: Open `report.html` in a browser
2. **JSON Report**: Parse `report.json` programmatically
3. **API**: Fetch via `GET /api/v1/runs/{id}` with `summary` field

## Troubleshooting

### Common Issues

**"ANTHROPIC_API_KEY not set"**
```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

**"Failed to connect to database"**
```bash
# Check PostgreSQL is running
pg_isready

# Create database
createdb testforge
```

**"Playwright browsers not installed"**
```bash
npx playwright install chromium
```

**"Rate limit exceeded"**
- Reduce `--max-pages` value
- Wait and retry
- Check API key quota

### Debug Mode

Enable verbose logging:

```bash
LOG_LEVEL=debug go run cmd/demo/main.go --url="https://example.com"
```

## Next Steps

- Read the [API Documentation](../api/openapi.yaml)
- Explore the [Architecture](../architecture/ARCHITECTURE.md)
- Check the [Enterprise Roadmap](../architecture/ENTERPRISE_ROADMAP.md)

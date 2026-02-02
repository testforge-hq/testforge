# TestForge Architecture

## Overview

TestForge is a cloud-native, AI-powered test automation platform built using a modular microservices architecture. The system leverages Go for the core services, Temporal for workflow orchestration, and Claude AI for intelligent test generation and self-healing capabilities.

## Design Principles

1. **Domain-Driven Design**: Clear separation between domain models, services, and infrastructure
2. **Hexagonal Architecture**: Core business logic is independent of external systems
3. **Event-Driven**: Asynchronous processing using Temporal workflows
4. **Multi-Tenant**: Full tenant isolation with configurable resource limits
5. **AI-First**: LLM integration at every stage of the testing lifecycle

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Client Layer                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  Web Dashboard    │    CLI Tools     │    CI/CD Integrations    │    SDKs  │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              API Gateway                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  Authentication  │  Rate Limiting  │  Request Routing  │  CORS  │  Logging │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            API Server (Go + Chi)                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │   Tenant     │  │   Project    │  │   Test Run   │  │    Health    │    │
│  │   Handler    │  │   Handler    │  │   Handler    │  │   Handler    │    │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Service Layer                                   │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌────────────┐ │   │
│  │  │  Discovery  │  │ Test Design │  │ Script Gen  │  │  Execution │ │   │
│  │  │   Service   │  │   Service   │  │   Service   │  │   Service  │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └────────────┘ │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │   │
│  │  │   Healing   │  │  Reporting  │  │   Visual    │                 │   │
│  │  │   Service   │  │   Service   │  │   Service   │                 │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          │                           │                           │
          ▼                           ▼                           ▼
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│   PostgreSQL    │       │    Temporal     │       │     Redis       │
│   (Data Store)  │       │  (Workflows)    │       │    (Cache)      │
└─────────────────┘       └─────────────────┘       └─────────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          │                           │                           │
          ▼                           ▼                           ▼
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│   Claude API    │       │   Playwright    │       │   Visual AI     │
│   (LLM)         │       │   (Browser)     │       │   (VJEPA)       │
└─────────────────┘       └─────────────────┘       └─────────────────┘
```

## Component Details

### API Server

The API server is built with Go using the Chi router framework.

**Key Features:**
- RESTful API with OpenAPI 3.1 specification
- JWT and API key authentication
- Rate limiting (300 req/min per tenant)
- Request timeout (60s default)
- Comprehensive error handling

**Middleware Stack:**
1. RequestID - Unique request identification
2. RealIP - Client IP extraction
3. Recovery - Panic handling
4. Logging - Structured request logging
5. Timeout - Request timeout enforcement
6. CORS - Cross-origin resource sharing
7. RateLimit - Per-tenant rate limiting
8. Auth - Authentication and authorization

### Domain Layer

The domain layer defines core business entities and interfaces.

**Entities:**
- `Tenant` - Organization/customer
- `Project` - Test project configuration
- `TestRun` - Test execution instance
- `TestCase` - Individual test case
- `TestResult` - Test execution result

**Interfaces:**
- `TenantRepository` - Tenant data access
- `ProjectRepository` - Project data access
- `TestRunRepository` - Test run data access

### Service Layer

#### Discovery Service

Crawls web applications to discover testable elements.

```go
type Crawler struct {
    config     DiscoveryConfig
    browser    playwright.Browser
    storage    StorageClient
}

func (c *Crawler) Crawl(ctx context.Context, url string) (*AppModel, error)
```

**Features:**
- Playwright-based headless browsing
- JavaScript rendering support
- Form and button detection
- Link extraction and crawling
- Screenshot capture
- Progress callbacks

#### Test Design Service (Architect)

Uses Claude AI to design comprehensive test cases.

```go
type TestArchitect struct {
    client LLMClient
    config ArchitectConfig
}

func (a *TestArchitect) DesignTestSuite(ctx context.Context, input DesignInput) (*DesignResult, error)
```

**Features:**
- Intelligent test case generation
- BDD format (Given/When/Then)
- Priority-based categorization
- Security and accessibility tests
- Chunked processing for large apps

#### Script Generation Service

Converts test designs to executable Playwright scripts.

```go
type ScriptGenerator struct {
    config GeneratorConfig
}

func (g *ScriptGenerator) GenerateScripts(suite *TestSuite) (*Project, error)
```

**Features:**
- TypeScript code generation
- Page Object Model pattern
- Fixtures and utilities
- Configurable project structure

#### Execution Service

Runs generated tests using Playwright.

```go
type Executor struct {
    config ExecutorConfig
    runner *PlaywrightRunner
}

func (e *Executor) Execute(ctx context.Context, tests []*TestCase) (*ExecutionResult, error)
```

**Features:**
- Parallel test execution
- Screenshot and video capture
- Trace recording
- Retry logic for flaky tests

#### Self-Healing Service

Automatically repairs broken tests using AI.

```go
type RepairAgent struct {
    client LLMClient
    config RepairConfig
}

func (r *RepairAgent) RepairTest(ctx context.Context, failure *TestFailure) (*RepairResult, error)
```

**Features:**
- Failure analysis
- Alternative selector generation
- Test code patching
- Validation of repairs

#### Reporting Service

Generates comprehensive test reports.

```go
type ReportGenerator struct {
    client LLMClient
    config ReportConfig
}

func (g *ReportGenerator) GenerateReport(ctx context.Context, run *TestRun) (*Report, error)
```

**Features:**
- Execution summaries
- AI-generated insights
- Trend analysis
- Export formats (HTML, PDF, JSON)

### Workflow Orchestration (Temporal)

Temporal workflows coordinate the entire test run lifecycle.

```go
func TestRunWorkflow(ctx workflow.Context, input TestRunInput) (*TestRunResult, error) {
    // Stage 1: Discovery
    discoveryResult := workflow.ExecuteActivity(ctx, DiscoveryActivity, input.URL)

    // Stage 2: Test Design
    testPlan := workflow.ExecuteActivity(ctx, TestDesignActivity, discoveryResult)

    // Stage 3: Script Generation
    scripts := workflow.ExecuteActivity(ctx, ScriptGenActivity, testPlan)

    // Stage 4: Execution
    execResult := workflow.ExecuteActivity(ctx, ExecutionActivity, scripts)

    // Stage 5: Self-Healing (if failures)
    if execResult.HasFailures {
        healResult := workflow.ExecuteActivity(ctx, HealingActivity, execResult.Failures)
        // Re-execute healed tests
    }

    // Stage 6: Reporting
    report := workflow.ExecuteActivity(ctx, ReportingActivity, execResult)

    return &TestRunResult{Report: report}, nil
}
```

**Activities:**
- `DiscoveryActivity` - Web crawling
- `TestDesignActivity` - AI test generation
- `ScriptGenActivity` - Code generation
- `ExecutionActivity` - Test execution
- `HealingActivity` - Self-healing
- `ReportingActivity` - Report generation

### Data Layer

#### PostgreSQL Schema

```sql
-- Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(64) UNIQUE NOT NULL,
    plan VARCHAR(20) DEFAULT 'free',
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Projects
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    base_url VARCHAR(2048) NOT NULL,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Test Runs
CREATE TABLE test_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    target_url VARCHAR(2048),
    workflow_id VARCHAR(255),
    discovery_result JSONB,
    test_plan JSONB,
    summary JSONB,
    report_url VARCHAR(2048),
    triggered_by VARCHAR(255),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_projects_tenant ON projects(tenant_id);
CREATE INDEX idx_test_runs_project ON test_runs(project_id);
CREATE INDEX idx_test_runs_status ON test_runs(status);
```

#### Redis Usage

- **LLM Response Caching**: SHA256-based cache keys with TTL
- **Rate Limiting**: Sliding window counters
- **Session Storage**: JWT token blacklisting

### LLM Integration

The LLM client provides a unified interface to Claude AI.

```go
type ClaudeClient struct {
    apiKey      string
    model       string
    rateLimiter *rate.Limiter
    cache       *LRUCache
}

func (c *ClaudeClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
```

**Features:**
- Rate limiting (configurable RPM)
- Response caching (LRU with SHA256 keys)
- Token counting
- Cost estimation
- Retry with exponential backoff

### Visual Testing Service

Python microservice using VJEPA for visual comparison.

```python
class VisualAIService:
    def compare_screenshots(self, baseline: bytes, current: bytes) -> ComparisonResult:
        """Compare two screenshots using VJEPA embeddings."""
        pass

    def detect_anomalies(self, screenshot: bytes) -> List[Anomaly]:
        """Detect visual anomalies in a screenshot."""
        pass
```

**Features:**
- VJEPA embedding generation
- Cosine similarity comparison
- Anomaly detection
- gRPC communication

## Security

### Authentication

- JWT tokens with RS256 signing
- API keys for service accounts
- Tenant isolation via X-Tenant-ID header

### Authorization

- Role-based access control (RBAC)
- Resource-level permissions
- Tenant boundary enforcement

### Data Security

- Encryption at rest (PostgreSQL)
- TLS in transit
- Secrets management (environment variables)
- Audit logging

## Scalability

### Horizontal Scaling

- Stateless API servers
- Temporal worker scaling
- Database connection pooling
- Redis cluster support

### Performance Optimizations

- LLM response caching
- Parallel test execution
- Chunked processing for large apps
- Connection pooling

## Monitoring

### Metrics (Prometheus)

- Request latency histograms
- Error rate counters
- LLM token usage
- Test execution metrics

### Logging (Zap)

- Structured JSON logging
- Request tracing
- Error context

### Alerting

- Error rate thresholds
- Latency SLOs
- Resource utilization

## Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /api ./cmd/api

FROM alpine:3.18
COPY --from=builder /api /api
EXPOSE 8080
CMD ["/api"]
```

### Kubernetes

- Deployment manifests
- Service definitions
- ConfigMaps and Secrets
- Horizontal Pod Autoscaler
- Network Policies

## Future Enhancements

See [ENTERPRISE_ROADMAP.md](ENTERPRISE_ROADMAP.md) for planned features including:

- Distributed test execution
- Advanced analytics
- Custom LLM support
- Enterprise SSO
- Compliance features

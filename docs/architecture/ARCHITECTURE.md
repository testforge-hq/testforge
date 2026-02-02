# TestForge Architecture

## Overview

TestForge is a cloud-native, AI-powered test automation platform built using a modular microservices architecture. The system leverages Go for the core services, Temporal for workflow orchestration, Claude AI for intelligent test generation and self-healing, and Qdrant for semantic pattern matching.

## Design Principles

1. **Domain-Driven Design**: Clear separation between domain models, services, and infrastructure
2. **Hexagonal Architecture**: Core business logic is independent of external systems
3. **Event-Driven**: Asynchronous processing using Temporal workflows
4. **Multi-Tenant**: Full tenant isolation with configurable resource limits
5. **AI-First**: LLM integration at every stage of the testing lifecycle
6. **Network Effects**: Cross-tenant learning creates a competitive moat

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                    Client Layer                                      │
├─────────────────────────────────────────────────────────────────────────────────────┤
│  Web Dashboard  │  CLI Tools  │  GitHub Action  │  CI/CD Plugins  │  SDKs  │  API   │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                    API Gateway                                       │
├─────────────────────────────────────────────────────────────────────────────────────┤
│  Auth (JWT/SSO)  │  Rate Limiting  │  RBAC  │  Audit Logging  │  Circuit Breaker   │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    ▼                     ▼                     ▼
┌───────────────────────────┐ ┌───────────────────────┐ ┌───────────────────────────┐
│      API Server (Go)      │ │    Admin Dashboard    │ │    Webhook Handlers       │
├───────────────────────────┤ ├───────────────────────┤ ├───────────────────────────┤
│ • Tenant/Project/Run APIs │ │ • Platform Overview   │ │ • Stripe Webhooks         │
│ • Auth & SSO Handlers     │ │ • User/Tenant Mgmt    │ │ • GitHub Webhooks         │
│ • Billing APIs            │ │ • Cost Analytics      │ │ • CI/CD Webhooks          │
│ • Badge/Share APIs        │ │ • System Health       │ │                           │
└───────────────────────────┘ └───────────────────────┘ └───────────────────────────┘
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    ▼                     ▼                     ▼
┌───────────────────────────┐ ┌───────────────────────┐ ┌───────────────────────────┐
│    Service Layer          │ │   Intelligence Layer  │ │    Billing Layer          │
├───────────────────────────┤ ├───────────────────────┤ ├───────────────────────────┤
│ • Discovery Service       │ │ • Knowledge Base      │ │ • Stripe Integration      │
│ • Test Design (AI)        │ │ • Pattern Repository  │ │ • Subscription Management │
│ • Script Generation       │ │ • Suggestion Engine   │ │ • Usage Tracking          │
│ • Execution Service       │ │ • Embedding Service   │ │ • Metered Billing         │
│ • Self-Healing (AI)       │ │ • Semantic Cache      │ │ • Invoice Management      │
│ • Reporting Service       │ │                       │ │                           │
└───────────────────────────┘ └───────────────────────┘ └───────────────────────────┘
                                          │
          ┌───────────────────────────────┼───────────────────────────────┐
          │                               │                               │
          ▼                               ▼                               ▼
┌─────────────────────┐       ┌─────────────────────┐       ┌─────────────────────┐
│     PostgreSQL      │       │      Temporal       │       │       Redis         │
│   (Primary Data)    │       │    (Workflows)      │       │   (Cache/Queue)     │
├─────────────────────┤       ├─────────────────────┤       ├─────────────────────┤
│ • Tenants/Users     │       │ • Test Workflows    │       │ • LLM Cache         │
│ • Projects/Runs     │       │ • Healing Workflows │       │ • Embedding Cache   │
│ • Subscriptions     │       │ • Billing Jobs      │       │ • Rate Limits       │
│ • Audit Logs        │       │ • Report Generation │       │ • Sessions          │
│ • Knowledge Base    │       │                     │       │ • Real-time Metrics │
└─────────────────────┘       └─────────────────────┘       └─────────────────────┘
          │                               │                               │
          └───────────────────────────────┼───────────────────────────────┘
                                          │
          ┌───────────────────────────────┼───────────────────────────────┐
          │                               │                               │
          ▼                               ▼                               ▼
┌─────────────────────┐       ┌─────────────────────┐       ┌─────────────────────┐
│    Claude API       │       │      Qdrant         │       │       MinIO         │
│   (LLM Provider)    │       │  (Vector Database)  │       │  (Artifact Storage) │
├─────────────────────┤       ├─────────────────────┤       ├─────────────────────┤
│ • Test Generation   │       │ • Pattern Vectors   │       │ • Screenshots       │
│ • Self-Healing      │       │ • Semantic Search   │       │ • Test Reports      │
│ • Report Insights   │       │ • Similarity Match  │       │ • Videos/Traces     │
└─────────────────────┘       └─────────────────────┘       └─────────────────────┘
          │                               │                               │
          └───────────────────────────────┼───────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              Execution Layer (K8s)                                   │
├─────────────────────────────────────────────────────────────────────────────────────┤
│   ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐     │
│   │   Sandbox    │    │   Sandbox    │    │   Sandbox    │    │  Visual AI   │     │
│   │   Pod #1     │    │   Pod #2     │    │   Pod #N     │    │   Service    │     │
│   │ (Playwright) │    │ (Playwright) │    │ (Playwright) │    │   (VJEPA)    │     │
│   └──────────────┘    └──────────────┘    └──────────────┘    └──────────────┘     │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

## Component Details

### API Server

The API server is built with Go using the standard library's `net/http` with Chi router patterns.

**Key Features:**
- RESTful API with OpenAPI 3.1 specification
- JWT, API key, and SSO (OIDC) authentication
- Role-Based Access Control (RBAC)
- Rate limiting with tenant-specific overrides
- Circuit breaker for external service resilience
- Comprehensive audit logging

**Middleware Stack:**
1. **RequestID** - Unique request identification
2. **RealIP** - Client IP extraction
3. **Recovery** - Panic handling
4. **Logging** - Structured request logging (Zap)
5. **Timeout** - Request timeout enforcement
6. **CORS** - Cross-origin resource sharing
7. **RateLimit** - Per-tenant rate limiting with Redis
8. **Auth** - JWT/API key/SSO authentication
9. **RBAC** - Permission enforcement
10. **Audit** - Action logging

### Authentication & Authorization

#### Authentication Methods

```go
// JWT Authentication
type SessionManager struct {
    accessTTL  time.Duration  // 15 minutes
    refreshTTL time.Duration  // 7 days
    signingKey []byte
}

// API Key Authentication
type APIKeyRepository struct {
    db    *sqlx.DB
    redis *redis.Client  // 5-minute cache
}

// SSO/OIDC Integration
type OIDCClient struct {
    providers map[string]*OIDCProvider  // Google, Okta, Azure AD, GitHub
}
```

**Supported Providers:**
- Google Workspace
- Okta
- Azure AD
- GitHub

#### RBAC System

```go
type Permission struct {
    Resource string   // "project", "testrun", "billing"
    Actions  []string // "read", "write", "delete", "execute"
}

type Role struct {
    Name        string
    Permissions []Permission
    IsSystem    bool
}
```

**System Roles:**
| Role | Permissions |
|------|-------------|
| admin | `*` (all permissions) |
| developer | `project:*`, `run:*`, `report:read` |
| viewer | `project:read`, `run:read`, `report:read` |

### Domain Layer

**Core Entities:**
- `Tenant` - Organization/customer with subscription
- `User` - Individual user with role assignments
- `Project` - Test project configuration
- `TestRun` - Test execution instance
- `Subscription` - Billing subscription (Stripe)
- `KnowledgeEntry` - Learned pattern for AI

**Key Interfaces:**
```go
type TenantRepository interface {
    Create(ctx context.Context, tenant *Tenant) error
    GetByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
    GetBySlug(ctx context.Context, slug string) (*Tenant, error)
    Update(ctx context.Context, tenant *Tenant) error
    Delete(ctx context.Context, id uuid.UUID) error
}
```

### Service Layer

#### Discovery Service

AI-enhanced crawler that discovers testable elements.

```go
type AICrawler struct {
    playwright   playwright.Browser
    llmClient    *ClaudeClient
    orchestrator *CrawlOrchestrator
}

func (c *AICrawler) Crawl(ctx context.Context, url string) (*AppModel, error)
```

**Features:**
- Playwright-based headless browsing
- AI-powered element classification
- Form and interaction detection
- Business flow inference
- Screenshot capture at key points
- Progress callbacks

#### Test Design Service

Uses Claude AI to design comprehensive test cases.

```go
type TestArchitect struct {
    client         *CachedClaudeClient  // With token caching
    knowledgeBase  *KnowledgeBase
    patterns       *PatternRepository
}
```

**Features:**
- Intelligent test case generation
- BDD format (Given/When/Then)
- Industry-specific templates
- Priority-based categorization
- Pattern-based suggestions

#### Self-Healing Service

Automatically repairs broken tests using AI and learned patterns.

```go
type RepairAgent struct {
    client      *ClaudeClient
    patterns    *PatternRepository
    qdrant      *QdrantClient
}

func (r *RepairAgent) RepairTest(ctx context.Context, failure *TestFailure) (*RepairResult, error)
```

**Healing Strategies:**
1. **Text-based** - Find by visible text
2. **Attribute-based** - Use stable attributes (data-testid, aria-label)
3. **Relative** - Find relative to stable elements
4. **Visual** - AI-powered visual matching
5. **Semantic** - Vector similarity from learned patterns

### Intelligence Layer

#### Knowledge Base

Cross-tenant learning system that creates a competitive moat.

```go
type KnowledgeBase struct {
    db       *sqlx.DB
    qdrant   *QdrantClient
    embedder *EmbeddingService
}

type KnowledgeEntry struct {
    Type          KnowledgeType  // healing, element, flow
    Pattern       string         // Anonymized pattern
    SuccessCount  int64
    Confidence    float64
    ContributedBy int            // Number of unique tenants
}
```

**Knowledge Types:**
- **Healing Patterns** - Successful selector repairs
- **Element Patterns** - Common UI element patterns
- **Flow Patterns** - User journey patterns by industry

#### Qdrant Integration

Vector database for semantic similarity search.

```go
type QdrantClient struct {
    config   QdrantConfig
    client   *http.Client
}

func (q *QdrantClient) SearchSimilar(ctx context.Context, embedding []float32, filter map[string]interface{}, limit int) ([]SearchResult, error)
```

**Use Cases:**
- Similar healing pattern lookup
- Semantic prompt caching
- Element pattern matching

#### Suggestion Engine

AI-powered suggestions for test improvements.

```go
type SuggestionEngine struct {
    kb       *KnowledgeBase
    patterns *PatternRepository
}

type Suggestion struct {
    Type        SuggestionType  // test, healing, optimization, coverage
    Priority    SuggestionPriority
    Title       string
    Description string
    Action      SuggestedAction
    Confidence  float64
}
```

### Billing Layer

#### Stripe Integration

```go
type StripeClient struct {
    secretKey  string
    webhookKey string
}

type SubscriptionService struct {
    db     *sqlx.DB
    stripe *StripeClient
}
```

**Features:**
- Subscription lifecycle management
- Plan tiers (Free, Pro, Enterprise)
- Metered billing for usage
- Stripe Checkout integration
- Customer portal

**Pricing Model:**
| Plan | Price | Test Runs | AI Tokens | Features |
|------|-------|-----------|-----------|----------|
| Free | $0/mo | 50/mo | 10K/mo | Basic reports |
| Pro | $99/mo | 500/mo | 100K/mo | Self-healing, Visual AI |
| Enterprise | Custom | Unlimited | Unlimited | SSO, Audit, SLA |

#### Usage Tracking

```go
type UsageTracker struct {
    buffer   map[string]*usageBuffer  // Buffered writes
    flusher  *backgroundFlusher
}

const (
    MetricTestRuns       = "test_runs"
    MetricAITokens       = "ai_tokens"
    MetricSandboxMinutes = "sandbox_minutes"
)
```

### Token Caching

Multi-layer caching to reduce LLM costs.

```go
type TokenCache struct {
    memCache   map[string]*cacheEntry  // LRU in-memory
    redis      *redis.Client           // Distributed cache
    costTracker *CostTracker
}

type CostTracker struct {
    dailyBudget   float64
    alertThreshold float64
}
```

**Cache Layers:**
1. **Memory** - LRU cache (10,000 entries, 1h TTL)
2. **Redis** - Distributed cache (24h TTL)
3. **Semantic** - Qdrant similarity (95% threshold)

**Cost Savings:**
- Cache hit rate: ~40-60%
- Estimated cost reduction: 30-50%

### Viral Features

#### Status Badges

```go
type BadgeService struct {
    cache map[string]*cachedBadge
}

func (bs *BadgeService) GenerateSVG(ctx context.Context, projectID uuid.UUID) ([]byte, error)
```

**Usage:**
```markdown
[![TestForge](https://api.testforge.io/badges/PROJECT_ID.svg)](https://testforge.io/projects/PROJECT_ID)
```

#### Shareable Reports

```go
type ShareableReport struct {
    ShareCode   string     // Unique public code
    IsPublic    bool
    Password    *string    // Optional protection
    ExpiresAt   *time.Time
    MaxViews    *int
}
```

**Features:**
- Public shareable URLs
- Password protection
- Expiration dates
- View limits
- Access analytics

#### GitHub Integration

```go
type GitHubIntegration struct {
    httpClient *http.Client
}

func (gh *GitHubIntegration) PostPRComment(ctx context.Context, req PRCommentRequest) error
func (gh *GitHubIntegration) CreateCheckRun(ctx context.Context, req CheckRunRequest) error
```

**Features:**
- PR comments with test results
- Check runs for CI/CD
- Automatic status updates
- GitHub Action available

### Admin Dashboard

Platform administration interface.

```go
type DashboardService struct {
    db    *sqlx.DB
    redis *redis.Client
}

type OverviewStats struct {
    TotalTenants    int64
    ActiveTenants   int64
    TotalUsers      int64
    RunningTests    int64
    MRR             float64
    ARR             float64
    PassRate        float64
}
```

**Capabilities:**
- Platform overview statistics
- Tenant/user management
- Running test monitoring
- Cost analytics
- System health checks
- Audit log viewing
- Feature flag management

### Workflow Orchestration (Temporal)

```go
func TestRunWorkflow(ctx workflow.Context, input TestRunInput) (*TestRunResult, error) {
    // Stage 1: Discovery (AI-enhanced)
    discoveryResult := workflow.ExecuteActivity(ctx, DiscoveryActivity, input.URL)

    // Stage 2: Test Design (with pattern suggestions)
    testPlan := workflow.ExecuteActivity(ctx, TestDesignActivity, discoveryResult)

    // Stage 3: Script Generation
    scripts := workflow.ExecuteActivity(ctx, ScriptGenActivity, testPlan)

    // Stage 4: Execution (sandboxed)
    execResult := workflow.ExecuteActivity(ctx, ExecutionActivity, scripts)

    // Stage 5: Self-Healing (with learned patterns)
    if execResult.HasFailures {
        healResult := workflow.ExecuteActivity(ctx, HealingActivity, execResult.Failures)
        // Learn from healing for future use
        workflow.ExecuteActivity(ctx, LearnPatternsActivity, healResult)
    }

    // Stage 6: Reporting
    report := workflow.ExecuteActivity(ctx, ReportingActivity, execResult)

    // Stage 7: Usage tracking
    workflow.ExecuteActivity(ctx, UsageTrackingActivity, input.TenantID, execResult)

    return &TestRunResult{Report: report}, nil
}
```

### Data Layer

#### PostgreSQL Schema

Key tables (see migrations for full schema):

```sql
-- Core
tenants, users, projects, test_runs

-- Auth & RBAC
api_keys, roles, tenant_memberships, sessions

-- Billing
subscriptions, usage_records, invoices, payment_methods

-- Intelligence
knowledge_entries, knowledge_contributions, flow_templates, suggestions

-- Audit
audit_logs, admin_audit_logs

-- Viral
shareable_reports, github_integrations, referrals
```

#### Redis Usage

- **LLM Response Caching**: SHA256-based cache keys
- **Embedding Cache**: Vector embeddings
- **Rate Limiting**: Sliding window counters
- **Session Storage**: JWT refresh tokens
- **Real-time Metrics**: Stream-based metrics

### Security

#### Authentication
- JWT tokens with RS256 signing
- API keys with SHA256 hashing and scopes
- SSO/OIDC with major providers
- MFA support (TOTP)

#### Authorization
- Role-based access control (RBAC)
- Resource-level permissions
- Tenant boundary enforcement
- API key scopes

#### Data Security
- Encryption at rest (PostgreSQL TDE)
- TLS 1.3 in transit
- Secrets via Vault/K8s secrets
- PII anonymization in knowledge base
- Comprehensive audit logging

#### Resilience
- Circuit breaker for external APIs
- Rate limiting with backoff
- Graceful degradation
- Fallback to cached responses

### Deployment

#### K3s/K8s Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        K3s Cluster                               │
├─────────────────────────────────────────────────────────────────┤
│  Namespace: testforge                                            │
│  ├── testforge-api (Deployment, HPA)                            │
│  ├── testforge-worker (Deployment, HPA)                         │
│  └── visual-ai (Deployment)                                     │
├─────────────────────────────────────────────────────────────────┤
│  Namespace: testforge-temporal                                   │
│  ├── temporal-server (StatefulSet)                              │
│  └── temporal-ui (Deployment)                                   │
├─────────────────────────────────────────────────────────────────┤
│  Namespace: testforge-data                                       │
│  ├── postgresql (StatefulSet)                                   │
│  ├── redis (StatefulSet)                                        │
│  └── minio (StatefulSet)                                        │
├─────────────────────────────────────────────────────────────────┤
│  Namespace: testforge-sandbox                                    │
│  └── (Dynamic pods for test execution)                          │
└─────────────────────────────────────────────────────────────────┘
```

#### Multi-Region

- Primary region with PostgreSQL master
- Read replicas in secondary regions
- Global load balancer (Cloudflare/AWS)
- Region-aware routing

### Monitoring

#### Metrics (Prometheus)
- Request latency histograms
- Error rate counters
- LLM token usage and costs
- Test execution metrics
- Cache hit rates
- Circuit breaker state

#### Logging (Zap)
- Structured JSON logging
- Request tracing (correlation IDs)
- Error context and stack traces
- Audit trail

#### Alerting
- Error rate thresholds
- Latency SLOs
- Budget alerts
- System health degradation

### API Endpoints Summary

| Category | Endpoints |
|----------|-----------|
| Health | `/health`, `/ready` |
| Auth | `/auth/login`, `/auth/sso/*`, `/auth/logout` |
| Tenants | `/tenants`, `/tenants/{id}` |
| Projects | `/projects`, `/projects/{id}` |
| Test Runs | `/runs`, `/runs/{id}`, `/runs/{id}/cancel` |
| Billing | `/billing/plans`, `/billing/subscribe`, `/billing/portal`, `/billing/usage` |
| Badges | `/badges/{project_id}.svg` |
| Share | `/runs/{id}/share`, `/r/{code}` |
| Admin | `/admin/overview`, `/admin/tenants`, `/admin/users`, `/admin/costs` |
| Webhooks | `/webhooks/stripe`, `/webhooks/github` |

## Database Migrations

| Migration | Description |
|-----------|-------------|
| 001 | Initial schema (tenants, projects, runs) |
| 002 | Add test results and reports |
| 003 | Add settings and configurations |
| 004 | API keys with scopes and rate limits |
| 005 | RBAC (users, roles, memberships) |
| 006 | Audit logging |
| 007 | Billing (subscriptions, usage, invoices) |
| 008 | Intelligence (knowledge base, patterns) |
| 009 | Viral features (badges, shares, GitHub) |
| 010 | Admin dashboard tables |

## Future Enhancements

- [ ] Custom LLM provider support (OpenAI, local models)
- [ ] Distributed test execution across regions
- [ ] Advanced analytics and ML insights
- [ ] Mobile app testing support
- [ ] API testing capabilities
- [ ] Compliance features (SOC 2, HIPAA)

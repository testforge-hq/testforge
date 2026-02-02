# TestForge API Reference

Complete reference for the TestForge REST API.

## Base URL

```
http://localhost:8080/api/v1
```

## Authentication

All API endpoints (except health checks) require authentication.

### Bearer Token

```bash
curl -H "Authorization: Bearer <token>" ...
```

### API Key

```bash
curl -H "X-API-Key: <api-key>" ...
```

### Tenant Header

Most endpoints require a tenant context:

```bash
curl -H "X-Tenant-ID: <tenant-uuid>" ...
```

## Rate Limiting

- **Limit**: 300 requests per minute per tenant
- **Headers**:
  - `X-RateLimit-Limit`: Maximum requests per window
  - `X-RateLimit-Remaining`: Remaining requests

## Error Responses

All errors follow this format:

```json
{
  "status": "error",
  "code": "ERROR_CODE",
  "message": "Human-readable message",
  "details": { "optional": "context" }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_ID` | 400 | Invalid UUID format |
| `INVALID_REQUEST` | 400 | Malformed request body |
| `VALIDATION_ERROR` | 400 | Validation failed |
| `UNAUTHORIZED` | 401 | Authentication required |
| `FORBIDDEN` | 403 | Access denied |
| `NOT_FOUND` | 404 | Resource not found |
| `ALREADY_EXISTS` | 409 | Resource conflict |
| `INVALID_STATE` | 409 | Invalid operation for state |
| `QUOTA_EXCEEDED` | 429 | Limit exceeded |
| `INTERNAL_ERROR` | 500 | Server error |

---

## Health Endpoints

### GET /health

Basic health check. No authentication required.

**Response**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T10:00:00Z"
}
```

### GET /ready

Detailed readiness check. No authentication required.

**Response**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T10:00:00Z",
  "checks": {
    "database": { "status": "up", "latency_ms": 5 },
    "redis": { "status": "up", "latency_ms": 1 },
    "temporal": { "status": "up", "latency_ms": 10 }
  }
}
```

---

## Tenant Endpoints

### GET /tenants

List all tenants.

**Query Parameters**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number |
| `per_page` | integer | 20 | Items per page (max 100) |

**Response**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "My Company",
      "slug": "my-company",
      "plan": "pro",
      "settings": {
        "max_concurrent_runs": 10,
        "max_test_cases_per_run": 500,
        "retention_days": 90,
        "enable_self_healing": true,
        "enable_visual_testing": true
      },
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 1,
    "total_pages": 1
  }
}
```

### POST /tenants

Create a new tenant.

**Request Body**
```json
{
  "name": "My Company",
  "slug": "my-company",
  "plan": "pro"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name (max 255 chars) |
| `slug` | string | Yes | URL-friendly ID (4-64 chars, lowercase) |
| `plan` | string | No | `free`, `pro`, or `enterprise` (default: `free`) |

**Response**: `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My Company",
  "slug": "my-company",
  "plan": "pro",
  "settings": { ... },
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-01-15T10:00:00Z"
}
```

### GET /tenants/{id}

Get tenant by ID.

**Path Parameters**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uuid | Tenant ID |

### GET /tenants/slug/{slug}

Get tenant by slug.

**Path Parameters**
| Parameter | Type | Description |
|-----------|------|-------------|
| `slug` | string | Tenant slug |

### PUT /tenants/{id}

Update a tenant.

**Request Body**
```json
{
  "name": "Updated Name",
  "plan": "enterprise",
  "settings": {
    "max_concurrent_runs": 20,
    "enable_visual_testing": true
  }
}
```

All fields are optional.

### DELETE /tenants/{id}

Delete a tenant and all associated data.

**Response**
```json
{
  "deleted": true
}
```

---

## Project Endpoints

### GET /tenants/{tenant_id}/projects

List projects for a tenant.

**Path Parameters**
| Parameter | Type | Description |
|-----------|------|-------------|
| `tenant_id` | uuid | Tenant ID |

**Query Parameters**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number |
| `per_page` | integer | 20 | Items per page |

### POST /tenants/{tenant_id}/projects

Create a new project.

**Request Body**
```json
{
  "name": "E-Commerce Site",
  "description": "Main e-commerce application",
  "base_url": "https://example.com",
  "settings": {
    "default_browser": "chromium",
    "default_viewport": "desktop",
    "viewport_width": 1920,
    "viewport_height": 1080,
    "default_timeout_ms": 30000,
    "retry_failed_tests": 2,
    "parallel_workers": 4,
    "capture_screenshots": true,
    "capture_video": true,
    "capture_trace": false,
    "max_crawl_depth": 5,
    "exclude_patterns": ["/admin/*", "/api/*"],
    "respect_robots_txt": true
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Project name |
| `description` | string | No | Project description |
| `base_url` | string | Yes | Base URL for testing |
| `settings` | object | No | Project settings |

### GET /projects/{id}

Get project by ID.

### PUT /projects/{id}

Update a project.

### DELETE /projects/{id}

Delete a project and all test runs.

---

## Test Run Endpoints

### GET /projects/{project_id}/runs

List test runs for a project.

**Response**
```json
{
  "data": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
      "project_id": "660e8400-e29b-41d4-a716-446655440001",
      "status": "completed",
      "target_url": "https://example.com",
      "workflow_id": "test-run-770e8400",
      "discovery_result": {
        "pages": [...],
        "total_pages": 15,
        "total_forms": 5,
        "crawl_duration": 45000
      },
      "test_plan": {
        "test_cases": [...],
        "total_count": 50,
        "by_priority": {
          "critical": 5,
          "high": 15,
          "medium": 20,
          "low": 10
        }
      },
      "summary": {
        "total_tests": 50,
        "passed": 45,
        "failed": 3,
        "skipped": 0,
        "healed": 2,
        "duration": 180000,
        "pass_rate": 90.0,
        "heal_rate": 40.0
      },
      "report_url": "https://reports.testforge.io/770e8400",
      "triggered_by": "api",
      "started_at": "2024-01-15T10:00:00Z",
      "completed_at": "2024-01-15T10:05:00Z",
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:05:00Z"
    }
  ],
  "meta": { ... }
}
```

### POST /runs

Create and start a new test run.

**Request Body**
```json
{
  "project_id": "660e8400-e29b-41d4-a716-446655440001",
  "target_url": "https://staging.example.com"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `project_id` | uuid | Yes | Project to run tests for |
| `target_url` | string | No | Override project's base URL |

**Response**: `201 Created`

Returns the created test run with `status: "pending"`.

**Error: 429 Quota Exceeded**
```json
{
  "status": "error",
  "code": "QUOTA_EXCEEDED",
  "message": "Maximum concurrent runs exceeded",
  "details": {
    "limit": 5,
    "current": 5
  }
}
```

### GET /runs/{id}

Get test run details.

### POST /runs/{id}/cancel

Cancel an active test run.

**Response**: Updated test run with `status: "cancelled"`

**Error: 409 Invalid State**
```json
{
  "status": "error",
  "code": "INVALID_STATE",
  "message": "Test run is already in terminal state",
  "details": {
    "status": "completed"
  }
}
```

### DELETE /runs/{id}

Delete a test run (must be in terminal state).

**Error: 409 Cannot Delete Active**
```json
{
  "status": "error",
  "code": "INVALID_STATE",
  "message": "Cannot delete active test run. Cancel it first.",
  "details": {
    "status": "executing"
  }
}
```

---

## Data Types

### Tenant Settings

```json
{
  "max_concurrent_runs": 5,
  "max_test_cases_per_run": 100,
  "retention_days": 30,
  "enable_self_healing": true,
  "enable_visual_testing": true,
  "allowed_domains": ["example.com"],
  "webhook_secret": "secret123",
  "notify_on_complete": true,
  "notify_on_failure": true
}
```

### Project Settings

```json
{
  "auth_type": "bearer",
  "auth_config": {
    "token": "Bearer xxx"
  },
  "default_browser": "chromium",
  "default_viewport": "desktop",
  "viewport_width": 1920,
  "viewport_height": 1080,
  "default_timeout_ms": 30000,
  "retry_failed_tests": 2,
  "parallel_workers": 4,
  "capture_screenshots": true,
  "capture_video": true,
  "capture_trace": false,
  "max_crawl_depth": 5,
  "exclude_patterns": ["/admin/*"],
  "include_patterns": [],
  "respect_robots_txt": true,
  "custom_headers": {
    "X-Custom": "value"
  },
  "custom_cookies": [
    {
      "name": "session",
      "value": "abc123",
      "domain": "example.com",
      "path": "/",
      "secure": true,
      "http_only": true
    }
  ]
}
```

### Test Run Status Values

| Status | Description |
|--------|-------------|
| `pending` | Waiting to start |
| `discovering` | Crawling the website |
| `designing` | Generating test cases with AI |
| `automating` | Creating Playwright scripts |
| `executing` | Running tests |
| `healing` | Auto-repairing broken tests |
| `reporting` | Generating report |
| `completed` | Finished successfully |
| `failed` | Encountered an error |
| `cancelled` | Manually cancelled |

### Test Case Structure

```json
{
  "id": "tc-001",
  "name": "Verify user login with valid credentials",
  "description": "Tests the login flow with correct username and password",
  "type": "e2e",
  "priority": "critical",
  "category": "authentication",
  "given": "User is on the login page",
  "when": "User enters valid credentials and clicks login",
  "then": "User is redirected to dashboard",
  "steps": [
    {
      "action": "navigate",
      "selector": null,
      "value": "/login",
      "expected": "Login page is displayed"
    },
    {
      "action": "fill",
      "selector": "#email",
      "value": "user@example.com",
      "expected": "Email field is filled"
    },
    {
      "action": "fill",
      "selector": "#password",
      "value": "password123",
      "expected": "Password field is filled"
    },
    {
      "action": "click",
      "selector": "button[type=submit]",
      "value": null,
      "expected": "Form is submitted"
    },
    {
      "action": "assert",
      "selector": null,
      "value": "/dashboard",
      "expected": "URL contains /dashboard"
    }
  ],
  "tags": ["login", "auth", "smoke"],
  "target_url": "https://example.com/login",
  "estimated_duration": "30s"
}
```

---

## Webhooks

Configure webhooks to receive notifications about test run events.

### Webhook Payload

```json
{
  "event": "test_run.completed",
  "timestamp": "2024-01-15T10:05:00Z",
  "data": {
    "test_run_id": "770e8400-e29b-41d4-a716-446655440002",
    "project_id": "660e8400-e29b-41d4-a716-446655440001",
    "status": "completed",
    "summary": {
      "total_tests": 50,
      "passed": 45,
      "failed": 3,
      "pass_rate": 90.0
    },
    "report_url": "https://reports.testforge.io/770e8400"
  }
}
```

### Event Types

- `test_run.started`
- `test_run.completed`
- `test_run.failed`
- `test_run.cancelled`

### Signature Verification

```python
import hmac
import hashlib

def verify_signature(payload, signature, secret):
    expected = hmac.new(
        secret.encode(),
        payload.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

# TestForge Documentation Index

Welcome to the TestForge documentation. This index provides quick access to all documentation resources.

## Quick Links

- [README](../README.md) - Project overview and quick start
- [Getting Started Guide](guides/GETTING_STARTED.md) - Step-by-step setup instructions
- [API Reference](guides/API_REFERENCE.md) - Complete API documentation
- [Architecture](architecture/ARCHITECTURE.md) - System design and components
- [Enterprise Roadmap](architecture/ENTERPRISE_ROADMAP.md) - Future features

## API Documentation

### Interactive Documentation

- **Swagger UI**: Open [`docs/api/swagger.html`](api/swagger.html) in a browser
- **ReDoc**: Open [`docs/api/redoc.html`](api/redoc.html) in a browser

To serve documentation locally:

```bash
# Using Python
cd docs/api && python -m http.server 8000

# Using Node.js
cd docs/api && npx serve .

# Then open http://localhost:8000/swagger.html or http://localhost:8000/redoc.html
```

### OpenAPI Specification

- [OpenAPI 3.1 Spec](api/openapi.yaml) - Machine-readable API definition

## Examples

- [cURL Examples](examples/curl_examples.sh) - Command-line API examples
- [Postman Collection](examples/postman_collection.json) - Import into Postman

## Documentation Structure

```
docs/
├── INDEX.md                    # This file
├── api/
│   ├── openapi.yaml           # OpenAPI 3.1 specification
│   ├── swagger.html           # Swagger UI viewer
│   └── redoc.html             # ReDoc viewer
├── guides/
│   ├── GETTING_STARTED.md     # Setup and first steps
│   └── API_REFERENCE.md       # API usage reference
├── architecture/
│   ├── ARCHITECTURE.md        # System architecture
│   └── ENTERPRISE_ROADMAP.md  # Future roadmap
└── examples/
    ├── curl_examples.sh       # cURL examples
    └── postman_collection.json # Postman collection
```

## Key Concepts

### Entities

- **Tenant**: Organization/customer account with isolated data
- **Project**: Test project targeting a specific web application
- **Test Run**: Single execution of the test automation pipeline

### Pipeline Stages

1. **Discovery**: Crawl the target website to discover pages and flows
2. **Test Design**: AI generates comprehensive test cases
3. **Script Generation**: Creates Playwright TypeScript tests
4. **Execution**: Runs the generated tests
5. **Self-Healing**: AI repairs broken tests automatically
6. **Reporting**: Generates detailed test reports

### Authentication

All API requests require:
- `Authorization: Bearer <token>` or `X-API-Key: <key>`
- `X-Tenant-ID: <uuid>` for tenant context

## Getting Help

- **Issues**: https://github.com/testforge/testforge/issues
- **Email**: support@testforge.io
- **Docs**: https://docs.testforge.io

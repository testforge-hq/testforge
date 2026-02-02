# TestForge GitHub Action

Run AI-powered end-to-end tests with TestForge directly in your GitHub workflow.

## Quick Start

```yaml
name: Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run TestForge Tests
        uses: testforge/action@v1
        with:
          api-key: ${{ secrets.TESTFORGE_API_KEY }}
          project-id: your-project-id
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `api-key` | TestForge API key | Yes | - |
| `project-id` | TestForge project ID | Yes | - |
| `base-url` | TestForge API base URL | No | `https://api.testforge.io` |
| `wait-for-results` | Wait for test results before completing | No | `true` |
| `timeout` | Timeout in minutes for waiting for results | No | `30` |
| `post-pr-comment` | Post results as a PR comment | No | `true` |
| `fail-on-error` | Fail the action if tests fail | No | `true` |
| `environment` | Test environment (staging, production) | No | `staging` |
| `target-url` | Override target URL for testing | No | - |
| `variables` | JSON object of test variables | No | - |

## Outputs

| Output | Description |
|--------|-------------|
| `run-id` | TestForge run ID |
| `status` | Test run status (passed, failed, error) |
| `total-tests` | Total number of tests |
| `passed-tests` | Number of passed tests |
| `failed-tests` | Number of failed tests |
| `report-url` | URL to full test report |
| `duration` | Test run duration |

## Examples

### Basic Usage

```yaml
- uses: testforge/action@v1
  with:
    api-key: ${{ secrets.TESTFORGE_API_KEY }}
    project-id: ${{ secrets.TESTFORGE_PROJECT_ID }}
```

### Custom Target URL

Run tests against a preview deployment:

```yaml
- uses: testforge/action@v1
  with:
    api-key: ${{ secrets.TESTFORGE_API_KEY }}
    project-id: ${{ secrets.TESTFORGE_PROJECT_ID }}
    target-url: ${{ steps.deploy.outputs.preview-url }}
```

### With Test Variables

```yaml
- uses: testforge/action@v1
  with:
    api-key: ${{ secrets.TESTFORGE_API_KEY }}
    project-id: ${{ secrets.TESTFORGE_PROJECT_ID }}
    variables: |
      {
        "username": "test@example.com",
        "password": "${{ secrets.TEST_PASSWORD }}"
      }
```

### Continue on Failure

Run tests but don't fail the build if tests fail:

```yaml
- uses: testforge/action@v1
  with:
    api-key: ${{ secrets.TESTFORGE_API_KEY }}
    project-id: ${{ secrets.TESTFORGE_PROJECT_ID }}
    fail-on-error: 'false'

- name: Process Results
  if: always()
  run: |
    echo "Tests: ${{ steps.testforge.outputs.passed-tests }}/${{ steps.testforge.outputs.total-tests }} passed"
```

### Upload Artifacts

```yaml
- uses: testforge/action@v1
  id: tests
  with:
    api-key: ${{ secrets.TESTFORGE_API_KEY }}
    project-id: ${{ secrets.TESTFORGE_PROJECT_ID }}

- uses: actions/upload-artifact@v4
  if: always()
  with:
    name: test-results
    path: testforge-results/
```

### Matrix Testing

Test against multiple environments:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        environment: [staging, production]
    steps:
      - uses: testforge/action@v1
        with:
          api-key: ${{ secrets.TESTFORGE_API_KEY }}
          project-id: ${{ secrets.TESTFORGE_PROJECT_ID }}
          environment: ${{ matrix.environment }}
```

## PR Comments

When `post-pr-comment` is enabled (default), the action will post a comment on pull requests with test results:

```
## TestForge Results ✅

| Metric | Value |
|--------|-------|
| Total Tests | 42 |
| Passed | 42 |
| Failed | 0 |
| Duration | 2m 34s |

[View Full Report](https://testforge.io/runs/abc123)
```

The comment is updated on subsequent runs rather than creating new comments.

## Secrets

Store your TestForge API key as a repository secret:

1. Go to your repository Settings
2. Click "Secrets and variables" → "Actions"
3. Click "New repository secret"
4. Name: `TESTFORGE_API_KEY`
5. Value: Your API key from TestForge dashboard

## Badge

Add a badge to your README:

```markdown
[![TestForge](https://api.testforge.io/badges/YOUR_PROJECT_ID.svg)](https://testforge.io/projects/YOUR_PROJECT_ID)
```

## Support

- [Documentation](https://docs.testforge.io)
- [Issues](https://github.com/testforge/action/issues)
- [Discord](https://discord.gg/testforge)

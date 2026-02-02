#!/bin/bash
# TestForge Sandbox Entrypoint
# Runs Playwright tests and uploads results

set -e

echo "TestForge Sandbox Starting..."
echo "Test ID: ${TEST_ID:-unknown}"
echo "Working Directory: $(pwd)"

# Ensure test files exist
if [ ! -d "/app/tests" ] || [ -z "$(ls -A /app/tests 2>/dev/null)" ]; then
    echo "ERROR: No test files found in /app/tests"
    exit 1
fi

# Run Playwright tests
echo "Running Playwright tests..."
cd /app/tests

# Install dependencies if package.json exists
if [ -f "package.json" ]; then
    npm install --silent
fi

# Run tests with JSON reporter for structured output
npx playwright test \
    --reporter=json \
    --output=/app/results \
    ${PLAYWRIGHT_FLAGS:-} \
    2>&1 | tee /app/results/output.log

TEST_EXIT_CODE=${PIPESTATUS[0]}

echo "Tests completed with exit code: $TEST_EXIT_CODE"

# Copy screenshots and videos
if [ -d "test-results" ]; then
    cp -r test-results/* /app/screenshots/ 2>/dev/null || true
fi

# Signal completion
echo "SANDBOX_COMPLETE:$TEST_EXIT_CODE"
exit $TEST_EXIT_CODE

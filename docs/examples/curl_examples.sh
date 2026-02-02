#!/bin/bash
# TestForge API Examples
# Usage: Set environment variables and run individual commands

# Configuration
BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
API_KEY="${API_KEY:-your-api-key}"
TENANT_ID="${TENANT_ID:-}"

# Headers
AUTH_HEADER="Authorization: Bearer $API_KEY"
TENANT_HEADER="X-Tenant-ID: $TENANT_ID"
CONTENT_TYPE="Content-Type: application/json"

echo "=== TestForge API Examples ==="
echo "Base URL: $BASE_URL"
echo ""

# Health Check
echo "--- Health Check ---"
echo "curl $BASE_URL/health"
curl -s "$BASE_URL/../health" | jq .
echo ""

# Readiness Check
echo "--- Readiness Check ---"
echo "curl $BASE_URL/ready"
curl -s "$BASE_URL/../ready" | jq .
echo ""

# List Tenants
echo "--- List Tenants ---"
echo "curl -H \"$AUTH_HEADER\" $BASE_URL/tenants"
# curl -s -H "$AUTH_HEADER" "$BASE_URL/tenants" | jq .
echo ""

# Create Tenant
echo "--- Create Tenant ---"
cat << 'EOF'
curl -X POST "$BASE_URL/tenants" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "name": "My Company",
    "slug": "my-company",
    "plan": "pro"
  }'
EOF
echo ""

# Get Tenant by ID
echo "--- Get Tenant by ID ---"
cat << 'EOF'
curl "$BASE_URL/tenants/$TENANT_ID" \
  -H "Authorization: Bearer $API_KEY"
EOF
echo ""

# Get Tenant by Slug
echo "--- Get Tenant by Slug ---"
cat << 'EOF'
curl "$BASE_URL/tenants/slug/my-company" \
  -H "Authorization: Bearer $API_KEY"
EOF
echo ""

# Update Tenant
echo "--- Update Tenant ---"
cat << 'EOF'
curl -X PUT "$BASE_URL/tenants/$TENANT_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "name": "My Company Updated",
    "settings": {
      "max_concurrent_runs": 10,
      "enable_visual_testing": true
    }
  }'
EOF
echo ""

# Create Project
echo "--- Create Project ---"
cat << 'EOF'
curl -X POST "$BASE_URL/tenants/$TENANT_ID/projects" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -d '{
    "name": "E-Commerce Site",
    "description": "Main e-commerce application tests",
    "base_url": "https://example.com",
    "settings": {
      "default_browser": "chromium",
      "capture_screenshots": true,
      "capture_video": true,
      "max_crawl_depth": 5
    }
  }'
EOF
echo ""

# List Projects
echo "--- List Projects ---"
cat << 'EOF'
curl "$BASE_URL/tenants/$TENANT_ID/projects" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# Get Project
echo "--- Get Project ---"
cat << 'EOF'
curl "$BASE_URL/projects/$PROJECT_ID" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# Create Test Run
echo "--- Create Test Run ---"
cat << 'EOF'
curl -X POST "$BASE_URL/runs" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -d '{
    "project_id": "$PROJECT_ID",
    "target_url": "https://staging.example.com"
  }'
EOF
echo ""

# Get Test Run
echo "--- Get Test Run ---"
cat << 'EOF'
curl "$BASE_URL/runs/$RUN_ID" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# List Test Runs
echo "--- List Test Runs ---"
cat << 'EOF'
curl "$BASE_URL/projects/$PROJECT_ID/runs" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# Cancel Test Run
echo "--- Cancel Test Run ---"
cat << 'EOF'
curl -X POST "$BASE_URL/runs/$RUN_ID/cancel" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# Delete Test Run
echo "--- Delete Test Run ---"
cat << 'EOF'
curl -X DELETE "$BASE_URL/runs/$RUN_ID" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# Delete Project
echo "--- Delete Project ---"
cat << 'EOF'
curl -X DELETE "$BASE_URL/projects/$PROJECT_ID" \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Tenant-ID: $TENANT_ID"
EOF
echo ""

# Delete Tenant
echo "--- Delete Tenant ---"
cat << 'EOF'
curl -X DELETE "$BASE_URL/tenants/$TENANT_ID" \
  -H "Authorization: Bearer $API_KEY"
EOF
echo ""

echo "=== End of Examples ==="

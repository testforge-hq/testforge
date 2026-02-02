#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}TestForge Local Development Setup${NC}"
echo "=================================="
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    echo "Please install Go 1.22+ from https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version | grep -oP '\d+\.\d+' | head -1)
echo "Go version: $GO_VERSION"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed${NC}"
    echo "Please install Docker from https://docs.docker.com/get-docker/"
    exit 1
fi
echo "Docker: $(docker --version)"

if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo -e "${RED}Error: Docker Compose is not installed${NC}"
    exit 1
fi
echo "Docker Compose: available"

echo -e "${GREEN}✓ All prerequisites met${NC}"
echo ""

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo -e "${YELLOW}Creating .env file from template...${NC}"
    cp .env.example .env
    echo -e "${GREEN}✓ Created .env file${NC}"
else
    echo -e "${YELLOW}✓ .env file already exists${NC}"
fi

# Download Go dependencies
echo ""
echo -e "${YELLOW}Downloading Go dependencies...${NC}"
go mod download
go mod tidy
echo -e "${GREEN}✓ Dependencies downloaded${NC}"

# Start Docker services
echo ""
echo -e "${YELLOW}Starting Docker services...${NC}"
docker-compose up -d

echo ""
echo -e "${YELLOW}Waiting for services to be healthy...${NC}"

# Wait for PostgreSQL
echo -n "Waiting for PostgreSQL"
for i in {1..30}; do
    if docker exec testforge-postgres pg_isready -U testforge > /dev/null 2>&1; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 2
done

# Wait for Redis
echo -n "Waiting for Redis"
for i in {1..15}; do
    if docker exec testforge-redis redis-cli ping > /dev/null 2>&1; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 1
done

# Wait for Temporal
echo -n "Waiting for Temporal"
for i in {1..60}; do
    if curl -s http://localhost:8088 > /dev/null 2>&1; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 2
done

# Run migrations
echo ""
echo -e "${YELLOW}Running database migrations...${NC}"
for f in migrations/*.sql; do
    echo "  Applying $(basename $f)..."
    docker exec -i testforge-postgres psql -U testforge -d testforge < "$f" 2>/dev/null || true
done
echo -e "${GREEN}✓ Migrations applied${NC}"

# Build the API
echo ""
echo -e "${YELLOW}Building API server...${NC}"
go build -o bin/api ./cmd/api
echo -e "${GREEN}✓ API built${NC}"

# Show status
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Setup Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Services running:"
docker-compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || docker-compose ps
echo ""
echo "Available endpoints:"
echo "  • API:         http://localhost:8080"
echo "  • Temporal UI: http://localhost:8088"
echo "  • MinIO:       http://localhost:9001 (minioadmin/minioadmin)"
echo ""
echo "Quick start commands:"
echo "  make run           # Start the API server"
echo "  make test          # Run tests"
echo "  make docker-logs   # View service logs"
echo ""
echo "Test the API:"
echo '  curl http://localhost:8080/health'
echo ""

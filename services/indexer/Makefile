# SysArmor Indexer Makefile
# OpenSearch-based security event indexing and search service

# Docker parameters
COMPOSE_FILE=docker-compose.yml
PROJECT_NAME=sysarmor-indexer
OPENSEARCH_PORT=9201
DASHBOARDS_PORT=5602

# OpenSearch parameters
OPENSEARCH_USER=admin
OPENSEARCH_PASS=admin
OPENSEARCH_URL=http://localhost:$(OPENSEARCH_PORT)

.PHONY: help setup build up down restart status logs health test clean certs

# Default target
all: setup up

# Help
help:
	@echo "SysArmor Indexer - Available commands:"
	@echo ""
	@echo "🚀 Setup & Deployment:"
	@echo "  setup         - Generate certificates and prepare environment"
	@echo "  build         - Build Docker images"
	@echo "  up            - Start all services"
	@echo "  down          - Stop all services"
	@echo "  restart       - Restart all services"
	@echo "  all           - Setup and start services"
	@echo ""
	@echo "📊 Monitoring:"
	@echo "  status        - Check service status"
	@echo "  logs          - View service logs"
	@echo "  health        - Check OpenSearch cluster health"
	@echo ""
	@echo "🔧 Management:"
	@echo "  certs         - Generate SSL certificates"
	@echo "  templates     - Apply index templates"
	@echo "  test          - Run basic functionality tests"
	@echo "  clean         - Clean up containers and volumes"
	@echo ""
	@echo "🔍 OpenSearch Operations:"
	@echo "  indices       - List all indices"
	@echo "  users         - List all users"
	@echo "  search        - Search recent events"
	@echo ""
	@echo "📖 Documentation:"
	@echo "  docs          - Open documentation"
	@echo "  dashboards    - Open OpenSearch Dashboards"

# Generate SSL certificates
certs:
	@echo "🔐 Generating SSL certificates..."
	@chmod +x scripts/generate-certs.sh
	@./scripts/generate-certs.sh
	@echo "✅ SSL certificates generated"

# Setup environment
setup: certs
	@echo "🛠️  Setting up SysArmor Indexer environment..."
	@echo "✅ Environment setup completed"

# Build Docker images
build:
	@echo "🔨 Building Docker images..."
	docker compose -f $(COMPOSE_FILE) build
	@echo "✅ Docker images built"

# Start services
up:
	@echo "🚀 Starting SysArmor Indexer services..."
	docker compose -f $(COMPOSE_FILE) up -d
	@echo "✅ Services started"
	@echo "📖 OpenSearch API: $(OPENSEARCH_URL)"
	@echo "📊 OpenSearch Dashboards: http://localhost:$(DASHBOARDS_PORT)"
	@echo "👤 Default credentials: $(OPENSEARCH_USER)/$(OPENSEARCH_PASS)"

# Stop services
down:
	@echo "🛑 Stopping SysArmor Indexer services..."
	docker compose -f $(COMPOSE_FILE) down
	@echo "✅ Services stopped"

# Restart services
restart: down up
	@echo "🔄 Services restarted"

# Check service status
status:
	@echo "📊 Checking service status..."
	docker compose -f $(COMPOSE_FILE) ps

# View logs
logs:
	@echo "📋 Viewing service logs..."
	docker compose -f $(COMPOSE_FILE) logs -f

# View OpenSearch logs only
logs-opensearch:
	@echo "📋 Viewing OpenSearch logs..."
	docker compose -f $(COMPOSE_FILE) logs -f opensearch

# View Dashboards logs only
logs-dashboards:
	@echo "📋 Viewing Dashboards logs..."
	docker compose -f $(COMPOSE_FILE) logs -f opensearch-dashboards

# Health check
health:
	@echo "🏥 Checking OpenSearch cluster health..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) "$(OPENSEARCH_URL)/_cluster/health?pretty" || echo "❌ Health check failed"

# Apply index templates
templates:
	@echo "📋 Applying index templates..."
	@curl -X PUT "$(OPENSEARCH_URL)/_index_template/sysarmor-events" \
		-u "$(OPENSEARCH_USER):$(OPENSEARCH_PASS)" \
		-H "Content-Type: application/json" \
		-d @templates/sysarmor-events-template.json
	@echo "✅ Index templates applied"

# List indices
indices:
	@echo "📋 Listing all indices..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) "$(OPENSEARCH_URL)/_cat/indices?v"

# List users
users:
	@echo "👥 Listing all users..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) "$(OPENSEARCH_URL)/_plugins/_security/api/internalusers?pretty"

# Search recent events
search:
	@echo "🔍 Searching recent events..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) -X GET "$(OPENSEARCH_URL)/sysarmor-events-*/_search?pretty" \
		-H "Content-Type: application/json" \
		-d '{"query":{"range":{"@timestamp":{"gte":"now-1h"}}},"size":10,"sort":[{"@timestamp":{"order":"desc"}}]}'

# Run basic tests
test:
	@echo "🧪 Running basic functionality tests..."
	@echo "Testing OpenSearch connection..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) "$(OPENSEARCH_URL)/_cluster/health" > /dev/null && echo "✅ OpenSearch connection OK" || echo "❌ OpenSearch connection failed"
	@echo "Testing Dashboards connection..."
	@curl -s "http://localhost:$(DASHBOARDS_PORT)/api/status" > /dev/null && echo "✅ Dashboards connection OK" || echo "❌ Dashboards connection failed"
	@echo "Testing index template..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) "$(OPENSEARCH_URL)/_index_template/sysarmor-events" > /dev/null && echo "✅ Index template exists" || echo "⚠️  Index template not found"

# Test data ingestion
test-ingest:
	@echo "🧪 Testing data ingestion..."
	@curl -X POST "$(OPENSEARCH_URL)/sysarmor-events-$(shell date +%Y.%m.%d)/_doc" \
		-u "$(OPENSEARCH_USER):$(OPENSEARCH_PASS)" \
		-H "Content-Type: application/json" \
		-d @templates/sample-event.json
	@echo "✅ Test event ingested"

# Clean up
clean:
	@echo "🧹 Cleaning up..."
	docker compose -f $(COMPOSE_FILE) down -v
	docker system prune -f
	@echo "✅ Cleanup completed"

# Open documentation
docs:
	@echo "📖 Opening documentation..."
	@if command -v open >/dev/null 2>&1; then \
		open docs/README.md; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open docs/README.md; \
	else \
		echo "📖 Documentation: docs/README.md"; \
	fi

# Open OpenSearch Dashboards
dashboards:
	@echo "📊 Opening OpenSearch Dashboards..."
	@if command -v open >/dev/null 2>&1; then \
		open http://localhost:$(DASHBOARDS_PORT); \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open http://localhost:$(DASHBOARDS_PORT); \
	else \
		echo "📊 OpenSearch Dashboards: http://localhost:$(DASHBOARDS_PORT)"; \
	fi

# Development helpers
dev-setup: setup templates
	@echo "🔧 Development environment ready"

# Production deployment
prod-deploy: setup up templates
	@echo "🚀 Production deployment completed"
	@echo "⚠️  Remember to change default passwords in production!"

# Backup indices
backup:
	@echo "💾 Creating backup..."
	@curl -X PUT "$(OPENSEARCH_URL)/_snapshot/backup_repo" \
		-u "$(OPENSEARCH_USER):$(OPENSEARCH_PASS)" \
		-H "Content-Type: application/json" \
		-d '{"type":"fs","settings":{"location":"/usr/share/opensearch/backup"}}'
	@curl -X PUT "$(OPENSEARCH_URL)/_snapshot/backup_repo/snapshot_$(shell date +%Y%m%d_%H%M%S)" \
		-u "$(OPENSEARCH_USER):$(OPENSEARCH_PASS)" \
		-H "Content-Type: application/json" \
		-d '{"indices":"sysarmor-events-*","ignore_unavailable":true,"include_global_state":false}'
	@echo "✅ Backup created"

# Monitor performance
monitor:
	@echo "📈 Monitoring cluster performance..."
	@curl -s -u $(OPENSEARCH_USER):$(OPENSEARCH_PASS) "$(OPENSEARCH_URL)/_cluster/stats?pretty"

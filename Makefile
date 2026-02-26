.PHONY: help dev build build-full build-sea build-frontend \
	generate-openapi generate-frontend-api generate-typescript-sdk generate-laravel-sdk generate-all-sdks \
	docker-build docker-push \
	test typecheck lint lint-fix clean generate-jwt-keys db-migrate db-studio

# ===================== Configuration =====================
APP                = apps/flowcatalyst
TS_SDK_DIR         = clients/typescript-sdk
LARAVEL_SDK_DIR    = clients/laravel-sdk
OPENAPI_DIR        = $(APP)/openapi
ECR_REGISTRY       ?= 392314734354.dkr.ecr.eu-west-1.amazonaws.com

# ===================== Help =====================
help: ## Show all targets
	@echo ""
	@echo "FlowCatalyst TypeScript Build System"
	@echo "======================================"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ===================== Development =====================
dev: ## Start FlowCatalyst in dev mode
	cd $(APP) && pnpm dev

# ===================== Build =====================
build: ## Build FlowCatalyst
	pnpm --filter @flowcatalyst/flowcatalyst build

build-full: ## Build FlowCatalyst with frontend
	pnpm --filter @flowcatalyst/flowcatalyst build:full

build-sea: ## Build as standalone executable (SEA)
	pnpm --filter @flowcatalyst/flowcatalyst build:sea

build-frontend: ## Build platform frontend
	pnpm --filter @flowcatalyst/platform-frontend build

# ===================== OpenAPI & SDKs =====================
generate-openapi: build ## Extract OpenAPI spec from platform
	cd $(APP) && pnpm openapi:extract
	@echo "OpenAPI spec written to $(OPENAPI_DIR)/"

generate-typescript-sdk: generate-openapi ## Generate TypeScript SDK from OpenAPI
	@echo "Copying OpenAPI spec to TypeScript SDK..."
	mkdir -p $(TS_SDK_DIR)/openapi
	cp $(OPENAPI_DIR)/openapi.yaml $(TS_SDK_DIR)/openapi/
	cp $(OPENAPI_DIR)/openapi.json $(TS_SDK_DIR)/openapi/
	@echo "Generating TypeScript SDK..."
	cd $(TS_SDK_DIR) && npm run generate && npm run build
	@echo "TypeScript SDK generated successfully"

generate-laravel-sdk: generate-openapi ## Generate Laravel SDK from OpenAPI
	@echo "Copying OpenAPI spec to Laravel SDK..."
	mkdir -p $(LARAVEL_SDK_DIR)/openapi
	cp $(OPENAPI_DIR)/openapi.json $(LARAVEL_SDK_DIR)/openapi/
	@echo "Preparing OpenAPI for Jane..."
	cd $(LARAVEL_SDK_DIR) && php scripts/prepare-openapi.php
	@echo "Generating Laravel SDK..."
	cd $(LARAVEL_SDK_DIR) && vendor/bin/jane-openapi generate -c jane-openapi.php
	@echo "Laravel SDK generated successfully"

generate-all-sdks: generate-typescript-sdk generate-laravel-sdk ## Generate both TypeScript and Laravel SDKs

generate-frontend-api: generate-openapi ## Generate frontend API client from OpenAPI
	mkdir -p apps/platform-frontend/openapi
	cp $(OPENAPI_DIR)/openapi.yaml apps/platform-frontend/openapi/
	cd apps/platform-frontend && pnpm api:generate

# ===================== Docker =====================
docker-build: ## Build Docker image for FlowCatalyst
	docker build -f $(APP)/Dockerfile -t flowcatalyst .

docker-push: docker-build ## Push FlowCatalyst image to ECR
	docker tag flowcatalyst $(ECR_REGISTRY)/flowcatalyst:latest
	docker push $(ECR_REGISTRY)/flowcatalyst:latest

# ===================== Testing & Quality =====================
test: ## Run all tests
	pnpm --filter @flowcatalyst/flowcatalyst test

typecheck: ## TypeScript type checking (no build step needed)
	pnpm --filter @flowcatalyst/flowcatalyst typecheck

lint: ## Run linter
	pnpm lint

lint-fix: ## Run linter auto-fix
	pnpm lint:fix

# ===================== Utilities =====================
clean: ## Remove all build artifacts
	rm -rf $(APP)/dist apps/platform-frontend/dist node_modules

generate-jwt-keys: ## Generate RSA key pair for JWT signing
	@mkdir -p keys
	openssl genpkey -algorithm RSA -out keys/private.pem -pkeyopt rsa_keygen_bits:2048
	openssl rsa -pubout -in keys/private.pem -out keys/public.pem
	@echo "JWT keys generated in keys/"

db-migrate: ## Run Drizzle migrations
	cd $(APP) && pnpm db:migrate

db-studio: ## Open Drizzle Studio
	cd $(APP) && pnpm db:studio

install: ## Install all dependencies
	pnpm install

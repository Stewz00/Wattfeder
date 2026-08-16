GO       ?= go
GOFMT    ?= gofmt
DOCKER   ?= docker
PACKAGES ?= ./...
DEMO_SCENARIO ?= scenarios/demo.json
FAULT_SCENARIO ?= scenarios/unreliable-telemetry-day.json
IMAGE ?= wattfeder

.DEFAULT_GOAL := help

.PHONY: help run agent demo demo-faults demo-clean check verify validate fmt fmt-check mod-tidy-check mod-verify vet test build docker-build compose-up compose-down

help: ## Show the available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run one simulated day as fast as possible.
	@$(GO) run ./cmd/wattfeder -pace fast -intervals 24

agent: ## Run the edge agent until interrupted.
	@$(GO) run ./cmd/wattfeder -agent-id agent-001 -interval 5s

demo: ## Run the fixed local demo scenario.
	@command -v $(GO) >/dev/null 2>&1 || { printf 'Required tool not found: %s\n' "$(GO)"; exit 1; }
	@$(GO) run ./cmd/wattfeder -scenario $(DEMO_SCENARIO)

demo-faults: ## Run the unreliable-telemetry demo scenario.
	@command -v $(GO) >/dev/null 2>&1 || { printf 'Required tool not found: %s\n' "$(GO)"; exit 1; }
	@$(GO) run ./cmd/wattfeder -scenario $(FAULT_SCENARIO)

demo-clean: ## Remove state created by the demo.
	@printf 'No demo state was created.\n'

docker-build: ## Build the edge agent's container image.
	@$(DOCKER) build -t $(IMAGE) .

compose-up: ## Start the agent, Prometheus, and Jaeger locally.
	@$(DOCKER) compose up -d

compose-down: ## Stop the local Compose environment.
	@$(DOCKER) compose down

check: verify validate ## Run all verification and validation checks.

verify: fmt-check mod-tidy-check mod-verify ## Verify source formatting and module metadata.

validate: vet test build ## Validate the code with analysis, tests, and compilation.

fmt: ## Format all Go source files.
	@$(GOFMT) -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check: ## Check that all Go source files are formatted.
	@unformatted="$$(find . -type f -name '*.go' -not -path './vendor/*' -exec $(GOFMT) -l {} +)"; \
	if [ -n "$$unformatted" ]; then \
		printf 'The following Go files are not formatted:\n%s\n' "$$unformatted"; \
		printf 'Run `make fmt` to format them.\n'; \
		exit 1; \
	fi

mod-tidy-check: ## Check whether go.mod and go.sum are tidy.
	@$(GO) mod tidy -diff

mod-verify: ## Verify downloaded module dependencies.
	@$(GO) mod verify

vet: ## Run Go's static analyzer.
	@$(GO) vet $(PACKAGES)

test: ## Run all tests.
	@$(GO) test $(PACKAGES)

build: ## Compile all packages.
	@output_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$output_dir"' EXIT; \
	$(GO) build -o "$$output_dir/" $(PACKAGES)

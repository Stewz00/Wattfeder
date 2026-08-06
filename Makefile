GO       ?= go
GOFMT    ?= gofmt
PACKAGES ?= ./...

.DEFAULT_GOAL := help

.PHONY: help run check verify validate fmt fmt-check mod-tidy-check mod-verify vet test build

help: ## Show the available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run the Wattfeder application.
	@$(GO) run ./cmd/wattfeder

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

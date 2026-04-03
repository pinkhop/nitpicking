################################################################################
# Makefile
#
# This Makefile provides tasks for building, testing, linting, and security
# scanning the project. All tasks are designed to be self-contained and
# idempotent — running them multiple times produces the same result.
#
# Usage:
#     make build                    Build the binary (dev version)
#     make build VERSION=1.2.3      Build with a specific version baked in
#     make test                     Run unit tests (alias for test-units)
#     make lint                     Run all linters
#     make sec                      Run all security scanners
#     make coverage                 Generate unit test coverage report
#     make clean                    Remove all build and coverage artifacts
#
################################################################################

# The module path, used to construct the -ldflags -X path for version injection.
MODULE       := github.com/pinkhop/nitpicking

# The name of the binary to produce. Change this when you rename the project.
APP_NAME     := np

# Whether to compile the application with CGO or to create a static binary.
CGO_ENABLED  := 0

# Build output directory. All compiled artifacts go here so that `clean` has a
# single directory to remove and the project root stays tidy.
DIST_DIR     := dist

# Coverage output directory.
COVERAGE_DIR := coverage

# VERSION may be set on the command line to bake a release version into the
# binary. When unset, the binary reports "dev" (the default in version.go).
#
# Examples:
#     make build                    → version="dev"
#     make build VERSION=1.2.3      → version="1.2.3"
#     make build VERSION=$(git describe --tags --always --dirty)
VERSION ?=

# Construct ldflags only when VERSION is non-empty. This avoids passing an
# empty -X flag, which would be harmless but noisy.
ifneq ($(strip $(VERSION)),)
LDFLAGS := -ldflags "-X $(MODULE)/internal/wiring.version=$(VERSION)"
else
LDFLAGS :=
endif

# Build tags:
#   netgo     — use the pure-Go DNS resolver and network stack so the binary 
#               has no dependency on the system's C resolver (important for 
#               static builds and Alpine/scratch containers).
#   osusergo  — use the pure-Go user/group lookup implementation, removing 
#               another common source of CGO dependency.
BUILD_TAGS := -tags="netgo,osusergo"

# -trimpath removes local filesystem paths from the compiled binary, which
# improves reproducibility and avoids leaking build-machine paths into stack
# traces or debug info.
BUILD_FLAGS := $(BUILD_TAGS) -trimpath $(LDFLAGS)

# Default target. Prints help when you run `make` with no arguments.
.DEFAULT_GOAL := help

################################################################################
# BUILD
################################################################################

## build: Compile the application binary into dist/.
##
## Produces a statically-linkable binary using pure-Go network and user
## packages (netgo, usergo) and strips local filesystem paths (-trimpath)
## for reproducible builds.
##
## To embed a version string in the binary, pass VERSION on the command line:
##
##     make build VERSION=1.2.3
##
## The version is injected via -ldflags into internal/wiring.version. When VERSION
## is omitted, the binary defaults to "dev".
.PHONY: build
build:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_FLAGS) -o $(DIST_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)/

################################################################################
# TESTING
################################################################################

## test-units: Run unit tests (no external dependencies required).
##
## Unit tests exercise isolated components using test doubles for all
## collaborators. They run without databases, networks, or other external
## systems and form the foundation of the test pyramid.
.PHONY: test-units
test-units:
	go test -short ./...

## test-boundary: Run boundary tests (may require external systems).
##
## Boundary tests verify adapters against real external systems such as
## databases and message brokers. They are guarded by the "boundary" build
## tag so they do not run during normal development. The package path is
## scoped to directories containing boundary tests to avoid compiling and
## entering unit-only packages.
.PHONY: test-boundary
test-boundary:
	go test -tags=boundary -run Boundary ./internal/adapters/driven/storage/sqlite/

## test-blackbox: Run blackbox component tests (requires a fully configured environment).
##
## Blackbox component tests exercise the full application stack — HTTP
## endpoints, CLI commands, and all wired dependencies. They are guarded by
## the "blackbox" build tag and typically run in CI or a staging environment.
.PHONY: test-blackbox
test-blackbox:
	go test -tags=blackbox -run Blackbox ./test/blackbox/

## test: Run unit tests (alias for test-units).
.PHONY: test
test: test-units

################################################################################
# COVERAGE
################################################################################

## coverage: Generate an HTML coverage report from unit tests.
##
## Writes a coverage profile to coverage/coverage.out and converts it to an
## HTML report at coverage/coverage.html. Open the HTML file in a browser to
## inspect per-line coverage.
.PHONY: coverage
coverage:
	@mkdir -p $(COVERAGE_DIR)
	go test -short -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

################################################################################
# FORMATTING
################################################################################

## gofumpt: Reformat Go source files to match community formatting standards.
##
## gofumpt is a stricter superset of gofmt. It enforces consistent formatting
## rules adopted by the broader Go community (e.g., grouping imports, removing
## unnecessary blank lines). The -w flag writes changes in place.
.PHONY: gofumpt
gofumpt:
	go tool mvdan.cc/gofumpt -w .

## goimports: Organize and clean up Go import declarations.
##
## goimports adds missing imports, removes unused ones, and groups them
## according to community conventions (stdlib, then third-party). The -w flag
## writes changes in place.
.PHONY: goimports
goimports:
	go tool golang.org/x/tools/cmd/goimports -w .

## templ: Generate Go code from .templ template files.
##
## Runs the templ code generator, which compiles .templ files into Go
## source files. This target must be run before build when templates
## have been modified.
.PHONY: templ
templ:
	go tool github.com/a-h/templ/cmd/templ generate

## fmt: Run all formatters (gofumpt, goimports).
##
## Convenience target that applies all automatic formatting fixes. Run this
## before committing to ensure consistent style.
.PHONY: fmt
fmt: gofumpt goimports

################################################################################
# LINTING
################################################################################

## lint-gofumpt: Check for formatting deviations from community standards.
##
## Runs gofumpt in diff mode (-d). Non-empty output means files need
## reformatting. Run `make gofumpt` to fix.
.PHONY: lint-gofumpt
lint-gofumpt:
	@echo "Checking gofumpt..."
	@diff=$$(go tool mvdan.cc/gofumpt -d .); \
	if [ -n "$$diff" ]; then \
		echo "$$diff"; \
		echo ""; \
		echo "Run 'make gofumpt' to fix formatting."; \
		exit 1; \
	fi

## lint-goimports: Check for import organization deviations from standards.
##
## Runs goimports in diff mode (-d). Non-empty output means imports need
## cleaning. Run `make goimports` to fix.
.PHONY: lint-goimports
lint-goimports:
	@echo "Checking goimports..."
	@diff=$$(go tool golang.org/x/tools/cmd/goimports -d .); \
	if [ -n "$$diff" ]; then \
		echo "$$diff"; \
		echo ""; \
		echo "Run 'make goimports' to fix imports."; \
		exit 1; \
	fi

## lint-ineffassign: Detect assignments to variables that are never read.
##
## Ineffective assignments usually indicate a logic error — the developer
## intended to use a value but never did, or assigned to the wrong variable.
.PHONY: lint-ineffassign
lint-ineffassign:
	@echo "Checking ineffassign..."
	go tool github.com/gordonklaus/ineffassign ./...

## lint-errcheck: Detect unchecked error return values.
##
## Go functions that return errors expect callers to check them. Ignoring an
## error silently discards failure information and can lead to subtle bugs.
## errcheck finds calls where the error is not assigned or inspected. The
## -ignoregenerated flag skips files containing the "Code generated ... DO NOT
## EDIT" comment.
.PHONY: lint-errcheck
lint-errcheck:
	@echo "Checking errcheck..."
	go tool github.com/kisielk/errcheck -ignoregenerated ./...

## lint-staticcheck: Run advanced static analysis on the codebase.
##
## staticcheck is a state-of-the-art Go linter that uses static analysis to
## find bugs, performance issues, and style violations. It covers checks
## beyond what `go vet` provides.
.PHONY: lint-staticcheck
lint-staticcheck:
	@echo "Checking staticcheck..."
	go tool honnef.co/go/tools/cmd/staticcheck ./...

## lint-vet: Run Go's built-in static analysis checks.
##
## go vet examines Go source code and reports suspicious constructs such as
## unreachable code, incorrect format strings, and misuse of sync primitives.
## It is fast and catches common mistakes that the compiler does not flag.
.PHONY: lint-vet
lint-vet:
	@echo "Checking go vet..."
	go vet ./...

## lint: Run all linters.
##
## Executes every lint-* target. Intended for CI pipelines and pre-commit
## checks. All linters must pass for the target to succeed.
.PHONY: lint
lint: lint-vet lint-gofumpt lint-goimports lint-ineffassign lint-errcheck lint-staticcheck

################################################################################
# SECURITY
################################################################################

## sec-gosec: Scan for security problems in Go source code.
##
## gosec inspects the Go AST and SSA representation to find common security
## issues such as SQL injection, hard-coded credentials, insecure random number
## generation, and unsafe use of crypto primitives. The -quiet flag suppresses
## informational output and shows only findings. The -exclude-generated flag
## skips files containing the "Code generated ... DO NOT EDIT" comment.
.PHONY: sec-gosec
sec-gosec:
	@echo "Running gosec..."
	go tool github.com/securego/gosec/v2/cmd/gosec -quiet -exclude-generated ./...

## sec-govulncheck: Check dependencies for known vulnerabilities.
##
## govulncheck analyzes your module's dependency graph and compiled code to
## find calls to functions in packages with known CVEs. Unlike simple
## dependency scanners, it reports only vulnerabilities whose affected symbols
## your code actually uses.
.PHONY: sec-govulncheck
sec-govulncheck:
	@echo "Running govulncheck..."
	go tool golang.org/x/vuln/cmd/govulncheck ./...

## sec: Run all security scanners.
##
## Executes every sec-* target. Intended for CI pipelines and pre-release
## verification.
.PHONY: sec
sec: sec-gosec sec-govulncheck

################################################################################
# CI
################################################################################

## ci: Run the full CI pipeline locally.
##
## Executes the build, all linters, security scanners, and unit tests in the
## same order a CI system would. Useful for verifying everything passes before
## pushing.
.PHONY: ci
ci: build lint sec test-units

################################################################################
# CLEANUP
################################################################################

## clean: Remove all build and coverage artifacts.
##
## Deletes the dist/ directory (compiled binaries), the coverage/ directory
## (coverage profiles and HTML reports), and the Go test cache. After running
## clean, the next build or test starts from a fresh state.
.PHONY: clean
clean:
	rm -rf $(DIST_DIR) $(COVERAGE_DIR)
	go clean -testcache

################################################################################
# HELP
################################################################################

## help: Show this help message.
##
## Parses "## target:" comments from the Makefile and displays them as a
## formatted help listing.
.PHONY: help
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## [-a-zA-Z0-9_]+:' $(MAKEFILE_LIST) | \
		sed 's/^## //' | \
		awk -F': ' '{printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build                    Build with default version (dev)"
	@echo "  make build VERSION=1.2.3      Build with version 1.2.3"
	@echo "  make lint                     Run all linters"
	@echo "  make ci                       Run full CI pipeline locally"

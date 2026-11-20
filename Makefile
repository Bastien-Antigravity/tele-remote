VERSION := $(strip $(shell cat VERSION.txt 2>/dev/null))
$(if $(VERSION),,$(error VERSION.txt is missing or empty))
LDFLAGS := -s -w -X 'main.ServerVersion=$(VERSION)'

.PHONY: all build test verify-standalone version clean

all: build

version:
	@echo $(VERSION)

verify-standalone:
	@echo "Verifying standalone Go build (GOWORK=off)..."
	GOWORK=off go build ./...

build:
	@echo "Building repository (version $(VERSION))..."
	@mkdir -p bin
	@if [ -f "go.mod" ]; then go build -ldflags="$(LDFLAGS)" -o bin/tele-remote ./cmd/tele-remote && go build -ldflags="$(LDFLAGS)" ./...; fi
	@if [ -f "Cargo.toml" ]; then cargo build --release; fi
	@if [ -f "setup.py" ] || [ -f "pyproject.toml" ]; then python3 -m build; fi

test:
	@echo "Running tests (version $(VERSION))..."
	@if [ -f "go.mod" ]; then go test -v ./...; fi
	@if [ -f "Cargo.toml" ]; then cargo test; fi
	@if [ -f "requirements.txt" ] || [ -f "pyproject.toml" ]; then pytest; fi

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf dist build *.egg-info target/

# Bugle — ECS Agent Framework with A2A Protocol
# Flywheel: build → lint → test before every commit

default: check

# Full quality gate (micro circuit stages 1-3)
check: build lint test

# Stage 1: Build
build:
    go build ./...

# Stage 2: Lint
lint:
    golangci-lint run ./...

# Stage 3: Unit test
test:
    go test ./... -count=1 -race -timeout 60s

# Stage 3b: Coverage
cover:
    go test ./... -coverprofile=coverage.out -timeout 60s
    go tool cover -func=coverage.out | tail -1
    @rm coverage.out

# Vet only (fast)
vet:
    go vet ./...

# Format
fmt:
    gofumpt -w .

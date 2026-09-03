set default-list := true

[windows]
set shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-Command"]

# Download module dependencies and verify the module cache.
setup:
    go mod download
    go mod verify

# Format all Go packages.
fmt:
    go fmt ./...

# Normalize go.mod and go.sum.
tidy:
    go mod tidy

# Verify downloaded module content.
verify:
    go mod verify

# Build all packages.
build:
    go build ./...

# Run Go's static analysis.
vet:
    go vet ./...

# Run the test suite once without cached results.
test:
    go test -count=1 ./...

# Run the test suite with the Go race detector.
test-race:
    go test -race -count=1 ./...

# Apply standard Go hygiene and run the normal validation suite.
check: fmt tidy verify build vet test

# Run the normal validation suite plus the race detector.
check-race: check test-race
set default-list := true
set minimum-version := "1.56.0"

[windows]
set shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-Command"]

msys2_root := 'C:\msys64'
race_cc := if os() == "windows" {
    msys2_root + '\ucrt64\bin\gcc.exe'
} else {
    'cc'
}
node_min_version := "22.19.0"
staticcheck_version := "v0.8.1"
govulncheck_version := "v1.1.4"

# Set up all Windows development and reviewer prerequisites.
[windows]
setup: setup-go setup-tools setup-race-windows setup-inspector-windows

# Set up Go tooling plus platform prerequisites used by check/inspect.
[unix]
setup: setup-go setup-tools setup-race-unix setup-inspector-unix

# Download and verify project module dependencies.
setup-go:
    go mod download
    go mod verify

# Download and verify the pinned analysis tools used by `just check`.
setup-tools:
    go run honnef.co/go/tools/cmd/staticcheck@{{staticcheck_version}} -version
    go run golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}} -version

# Install or verify the Windows toolchain required by Go's race detector.
[windows]
@setup-race-windows:
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw "winget is required for automated MSYS2 installation" }
    if (-not (Test-Path '{{msys2_root}}\usr\bin\bash.exe')) { Write-Host "Installing MSYS2..."; winget install --id MSYS2.MSYS2 --exact --accept-package-agreements --accept-source-agreements }
    if (-not (Test-Path '{{race_cc}}')) { Write-Host "Installing UCRT64 GCC..."; & '{{msys2_root}}\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'; & '{{msys2_root}}\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'; & '{{msys2_root}}\usr\bin\bash.exe' -lc 'pacman --noconfirm --needed -S mingw-w64-ucrt-x86_64-gcc' }
    if (-not (Test-Path '{{race_cc}}')) { throw "GCC was not found at {{race_cc}}" }
    & '{{race_cc}}' --version
    $library = (& '{{race_cc}}' --print-file-name libsynchronization.a).Trim(); if ($library -eq 'libsynchronization.a') { throw "Installed GCC does not satisfy Go's Windows race-detector requirement" }; Write-Host "Race runtime library: $library"

# Install or update Node.js LTS and verify npx for MCP Inspector.
[windows]
@setup-inspector-windows:
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw "winget is required for automated Node.js installation" }
    $node = Get-Command node.exe -ErrorAction SilentlyContinue; if (-not $node) { Write-Host "Installing Node.js LTS..."; winget install --id OpenJS.NodeJS.LTS --exact --accept-package-agreements --accept-source-agreements } elseif ([version]((& $node.Source --version).TrimStart('v')) -lt [version]'{{node_min_version}}') { Write-Host "Updating Node.js LTS for MCP Inspector compatibility..."; winget upgrade --id OpenJS.NodeJS.LTS --exact --accept-package-agreements --accept-source-agreements } else { Write-Host "Node.js already satisfies the MCP Inspector requirement." }
    $nodePath = (Get-Command node.exe -ErrorAction SilentlyContinue).Source; if (-not $nodePath) { $candidate = Join-Path $env:ProgramFiles 'nodejs\node.exe'; if (Test-Path $candidate) { $nodePath = $candidate } }; if (-not $nodePath) { throw "Node.js was not found after setup. Open a new PowerShell session and run 'just verify-inspector'." }; $verified = [version]((& $nodePath --version).TrimStart('v')); if ($verified -lt [version]'{{node_min_version}}') { throw "Node.js $verified is installed, but MCP Inspector requires {{node_min_version}} or newer" }; & $nodePath --version
    $npxPath = (Get-Command npx.cmd -ErrorAction SilentlyContinue).Source; if (-not $npxPath) { $candidate = Join-Path $env:ProgramFiles 'nodejs\npx.cmd'; if (Test-Path $candidate) { $npxPath = $candidate } }; if (-not $npxPath) { throw "npx was not found after Node.js setup. Open a new PowerShell session and try again." }; & $npxPath --version
    if (-not (Get-Command npx.cmd -ErrorAction SilentlyContinue)) { Write-Host "Node.js is installed, but this PowerShell session has not refreshed PATH. Open a new PowerShell session before running 'just inspect'." }

# Prepare or verify Unix race-detector prerequisites. macOS needs no separate C compiler setup.
[unix]
@setup-race-unix:
    if [ "$(uname -s)" = "Darwin" ]; then echo "macOS race detector ready; no separate C compiler setup is required."; else command -v cc >/dev/null 2>&1 || { echo "A C compiler is required for go test -race on this platform." >&2; exit 1; }; cc --version; fi

# Install or verify Node.js and npx for MCP Inspector on Unix. macOS uses Homebrew.
[unix]
@setup-inspector-unix:
    if [ "$(uname -s)" = "Darwin" ]; then command -v brew >/dev/null 2>&1 || { echo "Homebrew is required for automated Node.js setup on macOS." >&2; exit 1; }; if ! command -v node >/dev/null 2>&1; then echo "Installing Node.js with Homebrew..."; brew install node; elif ! node -e "const cur=process.versions.node.split('.').map(Number); const min='{{node_min_version}}'.split('.').map(Number); process.exit(cur[0]>min[0] || (cur[0]===min[0] && (cur[1]>min[1] || (cur[1]===min[1] && cur[2]>=min[2]))) ? 0 : 1)"; then if brew list --versions node >/dev/null 2>&1; then echo "Updating Node.js with Homebrew..."; brew upgrade node; else echo "Node.js is older than {{node_min_version}} and is not managed by Homebrew; update it manually." >&2; exit 1; fi; fi; fi
    node --version
    npx --version
    node -e "const cur=process.versions.node.split('.').map(Number); const min='{{node_min_version}}'.split('.').map(Number); const ok=cur[0]>min[0] || (cur[0]===min[0] && (cur[1]>min[1] || (cur[1]===min[1] && cur[2]>=min[2]))); if(!ok){console.error('Node.js {{node_min_version}} or newer is required for MCP Inspector'); process.exit(1)}"

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

# Run Go's built-in static analysis.
vet:
    go vet ./...

# Run the test suite once without cached results.
test:
    go test -count=1 ./...

# Exercise deterministic tests repeatedly to catch flakiness.
test-repeat:
    go test -count=10 ./...

# Run the test suite with Go's race detector on Windows.
[windows]
[env("CGO_ENABLED", "1")]
[env("CC", race_cc)]
test-race:
    go test -race -count=1 ./...

# Run the test suite with Go's race detector on Unix. Darwin does not need cgo or a C compiler.
[unix]
test-race:
    if [ "$(uname -s)" = "Darwin" ]; then go test -race -count=1 ./...; else CGO_ENABLED=1 CC=cc go test -race -count=1 ./...; fi

# Run the pinned Staticcheck release.
staticcheck:
    go run honnef.co/go/tools/cmd/staticcheck@{{staticcheck_version}} ./...

# Scan reachable code and required modules for known vulnerabilities.
vuln:
    go run golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}} ./...

# Run the complete local format, build, analysis, test, race, and security suite.
check: fmt tidy verify build vet test test-race test-repeat staticcheck vuln

# Run the MCP server using the demo catalog by default.
run config="examples/demo/catalog.json":
    go run ./cmd/agent-dependency-preflight -config "{{config}}"

# Verify the Node.js runtime used by the reference MCP Inspector.
verify-inspector:
    node --version
    npx --version
    node -e "const cur=process.versions.node.split('.').map(Number); const min='{{node_min_version}}'.split('.').map(Number); const ok=cur[0]>min[0] || (cur[0]===min[0] && (cur[1]>min[1] || (cur[1]===min[1] && cur[2]>=min[2]))); if(!ok){console.error('Node.js {{node_min_version}} or newer is required for MCP Inspector'); process.exit(1)}"

# Launch the reference MCP Inspector against the server and selected catalog.
inspect config="examples/demo/catalog.json": verify-inspector
    npx --yes @modelcontextprotocol/inspector go run ./cmd/agent-dependency-preflight -config "{{config}}"

# Run the deterministic local HTTP fixture used by the demo catalog.
demo-upstream:
    go run ./cmd/demo-upstream

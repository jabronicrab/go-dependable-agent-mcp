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

# Download module dependencies and verify the module cache.
setup:
    go mod download
    go mod verify

# Install and verify the Windows toolchain required by Go's race detector.
[windows]
[confirm("Install or update MSYS2 UCRT64 GCC for Go race-detector support? [y/N]")]
@setup-race-windows:
    Write-Host "Checking Windows package manager..."
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw "winget is required for automated MSYS2 installation" }

    if (-not (Test-Path '{{msys2_root}}\usr\bin\bash.exe')) { Write-Host "Installing MSYS2..."; winget install --id MSYS2.MSYS2 --exact --accept-package-agreements --accept-source-agreements } else { Write-Host "MSYS2 is already installed." }

    Write-Host "Updating MSYS2..."
    & '{{msys2_root}}\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'
    & '{{msys2_root}}\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'

    Write-Host "Installing UCRT64 GCC..."
    & '{{msys2_root}}\usr\bin\bash.exe' -lc 'pacman --noconfirm --needed -S mingw-w64-ucrt-x86_64-gcc'

    Write-Host "Verifying GCC..."
    if (-not (Test-Path '{{race_cc}}')) { throw "GCC was not found at {{race_cc}}" }
    & '{{race_cc}}' --version

    $library = (& '{{race_cc}}' --print-file-name libsynchronization.a).Trim(); if ($library -eq 'libsynchronization.a') { throw "Installed GCC does not satisfy Go's Windows race-detector requirement" }; Write-Host "Race runtime library: $library"

    Write-Host "Windows race-detector toolchain is ready."

# Install or update Node.js LTS and verify npx for the MCP Inspector demo.
[windows]
@setup-inspector-windows:
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw "winget is required for automated Node.js installation" }
    $node = Get-Command node.exe -ErrorAction SilentlyContinue; if (-not $node) { Write-Host "Installing Node.js LTS..."; winget install --id OpenJS.NodeJS.LTS --exact --accept-package-agreements --accept-source-agreements } elseif ([version]((& $node.Source --version).TrimStart('v')) -lt [version]'{{node_min_version}}') { Write-Host "Updating Node.js LTS for MCP Inspector compatibility..."; winget upgrade --id OpenJS.NodeJS.LTS --exact --accept-package-agreements --accept-source-agreements } else { Write-Host "Node.js already satisfies the MCP Inspector requirement." }
    $nodePath = (Get-Command node.exe -ErrorAction SilentlyContinue).Source; if (-not $nodePath) { $candidate = Join-Path $env:ProgramFiles 'nodejs\node.exe'; if (Test-Path $candidate) { $nodePath = $candidate } }; if (-not $nodePath) { throw "Node.js was not found after setup. Open a new PowerShell session and run 'just verify-inspector'." }; $verified = [version]((& $nodePath --version).TrimStart('v')); if ($verified -lt [version]'{{node_min_version}}') { throw "Node.js $verified is installed, but MCP Inspector requires {{node_min_version}} or newer" }; & $nodePath --version
    $npxPath = (Get-Command npx.cmd -ErrorAction SilentlyContinue).Source; if (-not $npxPath) { $candidate = Join-Path $env:ProgramFiles 'nodejs\npx.cmd'; if (Test-Path $candidate) { $npxPath = $candidate } }; if (-not $npxPath) { throw "npx was not found after Node.js setup. Open a new PowerShell session and try again." }; & $npxPath --version
    if (-not (Get-Command npx.cmd -ErrorAction SilentlyContinue)) { Write-Host "Node.js is installed, but this PowerShell session has not refreshed PATH. Open a new PowerShell session before running 'just inspector-demo'." }

# Perform the complete Windows developer and reviewer setup.
[windows]
setup-windows: setup setup-race-windows setup-inspector-windows

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

# Run the test suite with Go's race detector.
[env("CGO_ENABLED", "1")]
[env("CC", race_cc)]
test-race:
    go test -race -count=1 ./...

# Apply standard Go hygiene and run the normal validation suite.
check: fmt tidy verify build vet test

# Run the normal validation suite plus the race detector.
check-race: check test-race

# Run the deterministic local HTTP fixture used by the demo catalog.
demo-upstream:
    go run ./cmd/demo-upstream

# Run the MCP server over stdio using the deterministic demo catalog.
mcp-demo:
    go run ./cmd/agent-dependency-preflight -config examples/demo/catalog.json

# Verify the Node.js runtime used by the reference MCP Inspector.
verify-inspector:
    node --version
    npx --version
    node -e "const cur=process.versions.node.split('.').map(Number); const min='{{node_min_version}}'.split('.').map(Number); const ok=cur[0]>min[0] || (cur[0]===min[0] && (cur[1]>min[1] || (cur[1]===min[1] && cur[2]>=min[2]))); if(!ok){console.error('Node.js {{node_min_version}} or newer is required for MCP Inspector'); process.exit(1)}"

# Launch the reference MCP Inspector against the deterministic demo catalog.
inspector-demo: verify-inspector
    npx --yes @modelcontextprotocol/inspector go run ./cmd/agent-dependency-preflight -config examples/demo/catalog.json

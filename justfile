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

# Perform the complete Windows developer setup, including race-detector support.
[windows]
setup-windows: setup setup-race-windows

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
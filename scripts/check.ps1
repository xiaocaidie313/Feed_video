param([switch]$Integration)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $projectRoot 'backend')
try {
    $unformatted = @(gofmt -l cmd internal tests)
    if ($LASTEXITCODE -ne 0) { throw 'gofmt failed' }
    if ($unformatted.Count -gt 0) { throw "Run gofmt on: $($unformatted -join ', ')" }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'unit tests failed' }
    go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'build failed' }
    if ($Integration) {
        go test -tags integration ./tests/integration -count=1
        if ($LASTEXITCODE -ne 0) { throw 'integration tests failed' }
    }
}
finally { Pop-Location }

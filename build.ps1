# Build backend with vendored dependencies
Write-Host "Building AI Email Box Backend..." -ForegroundColor Cyan

go build -mod=vendor -o server.exe cmd/server/main.go

if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Build successful! Binary: server.exe" -ForegroundColor Green
} else {
    Write-Host "✗ Build failed!" -ForegroundColor Red
    exit 1
}

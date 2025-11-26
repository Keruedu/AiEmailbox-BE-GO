# Update vendored dependencies
Write-Host "Updating vendored dependencies..." -ForegroundColor Cyan

# Clean up and tidy
Write-Host "1. Tidying go.mod..." -ForegroundColor Yellow
go mod tidy

# Download dependencies
Write-Host "2. Downloading dependencies..." -ForegroundColor Yellow
go mod download

# Vendor dependencies
Write-Host "3. Vendoring dependencies..." -ForegroundColor Yellow
go mod vendor

# Verify
Write-Host "4. Verifying modules..." -ForegroundColor Yellow
go mod verify

if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Dependencies updated and vendored successfully!" -ForegroundColor Green
} else {
    Write-Host "✗ Failed to update dependencies!" -ForegroundColor Red
    exit 1
}

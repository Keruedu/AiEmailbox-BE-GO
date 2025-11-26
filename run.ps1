# Run backend server with vendored dependencies
Write-Host "Starting AI Email Box Backend..." -ForegroundColor Cyan
Write-Host "Server will run on http://localhost:8080" -ForegroundColor Yellow
Write-Host "Press Ctrl+C to stop" -ForegroundColor Yellow
Write-Host ""

go run -mod=vendor cmd/server/main.go

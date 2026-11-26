# Start PostgreSQL, Go API Server, and Vite Web UI in Docker, then start the Playwright Chrome Worker on host
Write-Host "Starting Docker services (postgres, server, web)..." -ForegroundColor Cyan
docker compose up -d --build

if ($LASTEXITCODE -ne 0) {
  Write-Host "Docker Compose failed. Please ensure Docker Desktop is running." -ForegroundColor Red
  exit 1
}

# Stop any existing worker process listening on port 3001 so startup never fails with EADDRINUSE
$existing = Get-NetTCPConnection -LocalPort 3001 -State Listen -ErrorAction SilentlyContinue
if ($existing) {
  Write-Host "Stopping existing worker process on port 3001 (PID $($existing.OwningProcess))..." -ForegroundColor Yellow
  Stop-Process -Id $existing.OwningProcess -Force -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 1
}

Write-Host "Docker services are UP! Web UI: http://localhost:5173/auto | API: http://localhost:8080" -ForegroundColor Green
Write-Host "Starting Playwright Chrome Worker on http://localhost:3001..." -ForegroundColor Cyan
Set-Location "$PSScriptRoot\worker"
npm start

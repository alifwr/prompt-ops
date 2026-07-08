# PromptOps — Development Startup Script (PowerShell)
# Run this script from the project root: ai-devops-paas/
# It starts all 3 services in parallel background jobs.

Write-Host ""
Write-Host "  ⚡ PromptOps — Starting All Services" -ForegroundColor Magenta
Write-Host "  ──────────────────────────────────────" -ForegroundColor DarkGray
Write-Host ""

# 1. AI Service (FastAPI + gRPC)
Write-Host "  [1/3] Starting AI Service (FastAPI + gRPC on :8000 / :50051)..." -ForegroundColor Cyan
$aiJob = Start-Job -Name "PromptOps-AI" -ScriptBlock {
    Set-Location $using:PWD\ai-service
    uv run python server.py
}

# Wait a moment for AI service to boot
Start-Sleep -Seconds 3

# 2. Gateway (Fastify on :3001)
Write-Host "  [2/3] Starting Gateway (Fastify on :3001)..." -ForegroundColor Cyan
$gatewayJob = Start-Job -Name "PromptOps-Gateway" -ScriptBlock {
    Set-Location $using:PWD\gateway
    npm run dev
}

# Wait a moment for gateway to boot
Start-Sleep -Seconds 3

# 3. Frontend (Nuxt on :3000)
Write-Host "  [3/3] Starting Frontend (Nuxt on :3000)..." -ForegroundColor Cyan
$frontendJob = Start-Job -Name "PromptOps-Frontend" -ScriptBlock {
    Set-Location $using:PWD\control-panel
    npm run dev
}

Write-Host ""
Write-Host "  ✅ All services starting!" -ForegroundColor Green
Write-Host ""
Write-Host "  Services:" -ForegroundColor White
Write-Host "    • Frontend:    http://localhost:3000" -ForegroundColor Gray
Write-Host "    • Gateway API: http://localhost:3001" -ForegroundColor Gray
Write-Host "    • AI Service:  http://localhost:8000" -ForegroundColor Gray
Write-Host "    • gRPC:        localhost:50051" -ForegroundColor Gray
Write-Host "    • Swagger:     http://localhost:3001/documentation" -ForegroundColor Gray
Write-Host ""
Write-Host "  Press Ctrl+C to stop all services." -ForegroundColor DarkGray
Write-Host ""

# Keep the script running and stream logs
try {
    while ($true) {
        # Check if jobs are still running
        $jobs = Get-Job -Name "PromptOps-*" 2>$null
        foreach ($job in $jobs) {
            if ($job.State -eq 'Failed') {
                Write-Host "  ⚠️  $($job.Name) failed!" -ForegroundColor Red
                Receive-Job $job
            }
        }
        Start-Sleep -Seconds 5
    }
}
finally {
    Write-Host "`n  Stopping all services..." -ForegroundColor Yellow
    Get-Job -Name "PromptOps-*" | Stop-Job -PassThru | Remove-Job
    Write-Host "  All services stopped." -ForegroundColor Green
}

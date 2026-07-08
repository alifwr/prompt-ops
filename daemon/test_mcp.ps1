# PromptOps Daemon MCP Stdio Test Runner
# Spawns daemon.exe with --stdio and pipes JSON-RPC commands into it, capturing stdout responses.

$daemonPath = ".\daemon.exe"
if (-not (Test-Path $daemonPath)) {
    Write-Host "Error: daemon.exe not found. Please compile the daemon first." -ForegroundColor Red
    exit 1
}

Write-Host "Starting PromptOps Daemon in stdio mode..." -ForegroundColor Cyan

# Define JSON-RPC request payloads (one payload per line)
$requestListTools = '{"jsonrpc": "2.0", "method": "tools/list", "id": 1}'
$requestGetStats = '{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "get_system_stats"}, "id": 2}'
$requestListContainers = '{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "list_containers", "arguments": {"all": true}}, "id": 3}'

# Start the process with redirected stdin/stdout
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $daemonPath
$psi.Arguments = "--stdio"
$psi.UseShellExecute = $false
$psi.RedirectStandardInput = $true
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true

$proc = New-Object System.Diagnostics.Process
$proc.StartInfo = $psi
[void]$proc.Start()

$stdin = $proc.StandardInput
$stdout = $proc.StandardOutput
$stderr = $proc.StandardError

# Helper function to send payload and read response
function Send-Payload($payload) {
    Write-Host "`n>>> SENDING: $payload" -ForegroundColor White
    $stdin.WriteLine($payload)
    
    # Read the single-line JSON-RPC response from stdout
    $response = $stdout.ReadLine()
    Write-Host "<<< RECEIVED: $response" -ForegroundColor Green
    return $response
}

# 1. Test tools/list
$resp1 = Send-Payload $requestListTools

# 2. Test get_system_stats
$resp2 = Send-Payload $requestGetStats

# 3. Test list_containers
$resp3 = Send-Payload $requestListContainers

# Close stdin to terminate the daemon process
$stdin.Close()
$proc.WaitForExit(5000)

# Check if there were any errors logged on stderr
$errLogs = $stderr.ReadToEnd()
if ($errLogs) {
    Write-Host "`n--- System stderr logs (for debugging) ---" -ForegroundColor Yellow
    Write-Host $errLogs -ForegroundColor Gray
}

Write-Host "`nMCP Stdio Verification completed successfully!" -ForegroundColor Cyan

# Starts cs2-watch against a remote game server: quick tunnel -> fresh ingest_url -> panel.
# Ctrl+C stops both. Needs cloudflared.exe next to this script.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$cfgPath = Join-Path $PSScriptRoot 'config.json'
$tunnelLog = Join-Path $env:TEMP 'cs2-watch-tunnel.log'

# Leftovers from a window closed with X (finally doesn't run then).
Get-Process cloudflared, cs2-watch -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -like "$PSScriptRoot\*" } | Stop-Process -Force

$tunnel = Start-Process .\cloudflared.exe -ArgumentList 'tunnel', '--url', 'http://127.0.0.1:8080' `
    -RedirectStandardError $tunnelLog -WindowStyle Hidden -PassThru
try {
    $url = $null
    for ($i = 0; $i -lt 60 -and -not $url; $i++) {
        Start-Sleep -Milliseconds 500
        $m = Select-String -Path $tunnelLog -Pattern 'https://[a-z0-9-]+\.trycloudflare\.com' -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($m) { $url = $m.Matches[0].Value }
    }
    if (-not $url) { throw "cloudflared gave no URL in 30s, see $tunnelLog" }

    # Regex edit keeps the file's formatting; WriteAllText writes UTF-8 without BOM (Go's JSON parser rejects a BOM).
    $cfg = [IO.File]::ReadAllText($cfgPath)
    $oldUrl = ([regex]'"ingest_url"\s*:\s*"([^"]*)"').Match($cfg).Groups[1].Value
    $token = ([regex]'"auth_token"\s*:\s*"([^"]*)"').Match($cfg).Groups[1].Value
    $cfg = $cfg -replace '"ingest_url"\s*:\s*"[^"]*"', "`"ingest_url`": `"$url/ingest`""
    [IO.File]::WriteAllText($cfgPath, $cfg)
    Write-Host "Tunnel: $url"

    $panel = Start-Process .\cs2-watch.exe -NoNewWindow -PassThru
    for ($i = 0; $i -lt 20; $i++) {
        Start-Sleep -Milliseconds 500
        $c = [Net.Sockets.TcpClient]::new()
        try { if ($c.ConnectAsync('127.0.0.1', 8080).Wait(300)) { break } } catch {} finally { $c.Dispose() }
    }

    # The server keeps POSTing to every URL ever registered; drop the previous run's dead one.
    if ($oldUrl -and $oldUrl -ne "$url/ingest") {
        $body = @{ command = "logaddress_del_http `"$oldUrl`"" } | ConvertTo-Json
        try {
            Invoke-RestMethod http://127.0.0.1:8080/api/rcon -Method Post -Body $body -ContentType 'application/json' `
                -Headers @{ Authorization = "Bearer $token" } | Out-Null
        } catch { Write-Warning "Could not remove old log address: $_" }
    }

    Start-Process 'http://127.0.0.1:8080'
    Wait-Process -Id $panel.Id
} finally {
    Stop-Process -Id $tunnel.Id -Force -ErrorAction SilentlyContinue
    if ($panel) { Stop-Process -Id $panel.Id -Force -ErrorAction SilentlyContinue }
}

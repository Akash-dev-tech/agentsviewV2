param([switch]$Clean, [switch]$Serve)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $repo '.env'
$dataDir = Join-Path $env:USERPROFILE '.agentsview'
if (-not (Test-Path -LiteralPath $envFile -PathType Leaf)) {
    throw 'Copy .env.example to .env and fill in your R2 settings.'
}
if (-not (Test-Path -LiteralPath (Join-Path $dataDir 'config.toml') -PathType Leaf)) {
    throw "Create $dataDir\config.toml using INSTRUCTIONS.md."
}

$required = @(
    'R2_BUCKET', 'AWS_ACCESS_KEY_ID', 'AWS_SECRET_ACCESS_KEY', 'AWS_ENDPOINT_URL',
    'AWS_REGION', 'AWS_REQUEST_CHECKSUM_CALCULATION',
    'AWS_RESPONSE_CHECKSUM_VALIDATION', 'AGENTSVIEW_S3_ONLY'
)
$allowed = $required + @('AV_WORKER_URL', 'AV_ADMIN_SECRET')
$values = @{}
# ReadAllLines removes CRLF and accepts UTF-8 files with a BOM.
$lineNumber = 0
foreach ($line in [IO.File]::ReadAllLines($envFile)) {
    $lineNumber++
    if ($line -match '^\s*(#|$)') { continue }
    if ($line -cnotmatch '^\s*([A-Z][A-Z0-9_]*)\s*=(.*)$' -or
        $Matches[1] -cnotin $allowed) {
        throw "Unsupported .env entry on line $lineNumber. Use the names in .env.example."
    }
    $values[$Matches[1]] = $Matches[2].Trim()
}
foreach ($name in $required) {
    if (-not $values[$name]) { throw "Set $name in .env." }
}
if ($values['AGENTSVIEW_S3_ONLY'] -ne '1') {
    throw 'Set AGENTSVIEW_S3_ONLY=1 in .env.'
}
if ($values['R2_BUCKET'] -cnotmatch '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$') {
    throw 'Set R2_BUCKET to the bucket name.'
}

$repoBinary = Join-Path $repo 'agentsview.exe'
if (Test-Path -LiteralPath $repoBinary -PathType Leaf) {
    $binary = $repoBinary
} else {
    $binary = (Get-Command agentsview.exe -CommandType Application -ErrorAction Stop).Source
}
$values['AGENTSVIEW_DATA_DIR'] = $dataDir
$values['AGENTSVIEW_NO_DAEMON'] = '1'
$values['AWS_S3_ENDPOINT'] = $values['AWS_ENDPOINT_URL']
$values['AWS_SESSION_TOKEN'] = $null
$previous = @{}

try {
    foreach ($name in $values.Keys) {
        $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $values[$name], 'Process')
    }

    & $binary daemon stop
    if ($LASTEXITCODE -ne 0) { throw "Daemon stop failed with exit code $LASTEXITCODE." }

    if ($Clean) {
        $databaseFiles = @('sessions.db', 'sessions.db-wal', 'sessions.db-shm', 'sessions.db-journal') |
            ForEach-Object { Join-Path $dataDir $_ } |
            Where-Object { Test-Path -LiteralPath $_ -PathType Leaf }
        if ($databaseFiles) {
            $backup = Join-Path $dataDir "sqlite-backup-$([guid]::NewGuid())"
            New-Item -ItemType Directory -Path $backup | Out-Null
            $moved = @()
            try {
                foreach ($file in $databaseFiles) {
                    Move-Item -LiteralPath $file -Destination $backup
                    $moved += $file
                }
            } catch {
                foreach ($file in $moved) {
                    Move-Item -LiteralPath (Join-Path $backup (Split-Path -Leaf $file)) -Destination $file
                }
                throw
            }
            Write-Host "Previous SQLite files moved to $backup"
        }
    }

    & $binary source sync --rescan
    if ($LASTEXITCODE -ne 0) { throw "Sync failed with exit code $LASTEXITCODE." }

    if ($Serve) {
        & $binary serve --host 127.0.0.1
        if ($LASTEXITCODE -ne 0) { throw "Dashboard exited with code $LASTEXITCODE." }
    }
} finally {
    foreach ($name in $previous.Keys) {
        [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
}

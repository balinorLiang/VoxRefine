param(
  [int]$Port = 55432,
  [switch]$Reset
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$asciiRoot = Join-Path $env:LOCALAPPDATA "polyphone-checker"
$bin = Join-Path $asciiRoot "pgsql\bin"
$data = Join-Path $asciiRoot "pgdata-ascii"
$log = Join-Path $asciiRoot "pg.log"

if ($env:PGBIN -and (Test-Path (Join-Path $env:PGBIN "initdb.exe"))) {
  $bin = $env:PGBIN
}

if (-not (Test-Path (Join-Path $bin "initdb.exe"))) {
  $fallback = Join-Path $root ".tools\pgsql\bin"
  if (Test-Path (Join-Path $fallback "initdb.exe")) {
    if ($fallback -match "[^\x00-\x7F]") {
      throw "PostgreSQL binaries are under a non-ASCII path ($fallback). Move them to $bin or use Docker."
    }
    $bin = $fallback
  } else {
    throw "PostgreSQL binaries not found. Run 'docker compose up -d', or install PostgreSQL 16 and set PGBIN, or extract the EDB Windows zip to $bin."
  }
}

$initdb = Join-Path $bin "initdb.exe"
$pgctl = Join-Path $bin "pg_ctl.exe"
$psql = Join-Path $bin "psql.exe"
$createdb = Join-Path $bin "createdb.exe"

if ($Reset -and (Test-Path $data)) {
  & $pgctl -D $data stop 2>$null | Out-Null
  Remove-Item -LiteralPath $data -Recurse -Force
}

if (-not (Test-Path (Join-Path $data "PG_VERSION"))) {
  New-Item -ItemType Directory -Force -Path $data | Out-Null
  & $initdb -D $data -U polyphone -A trust --encoding=UTF8 --locale=C
  if ($LASTEXITCODE -ne 0) { throw "initdb failed" }
}

& $pgctl -D $data status 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
  & $pgctl -D $data -l $log -o "-p $Port -h 127.0.0.1" start
  if ($LASTEXITCODE -ne 0) { throw "pg_ctl start failed. See $log" }
  Start-Sleep -Seconds 1
}

$exists = & $psql -h 127.0.0.1 -p $Port -U polyphone -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'polyphone'"
if ($exists -ne "1") {
  & $createdb -h 127.0.0.1 -p $Port -U polyphone polyphone
  if ($LASTEXITCODE -ne 0) { throw "createdb failed" }
}

Write-Host "PostgreSQL is ready."
Write-Host "DATABASE_URL=postgres://polyphone:polyphone@127.0.0.1:$Port/polyphone?sslmode=disable"

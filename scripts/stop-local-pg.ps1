$ErrorActionPreference = "Stop"
$asciiRoot = Join-Path $env:LOCALAPPDATA "polyphone-checker"
$pgctl = Join-Path $asciiRoot "pgsql\bin\pg_ctl.exe"
$data = Join-Path $asciiRoot "pgdata-ascii"
if ((Test-Path $pgctl) -and (Test-Path $data)) {
  & $pgctl -D $data stop
}

<#
.SYNOPSIS
  Deploys spotterd to a target Linux device over SSH.

.DESCRIPTION
  Uploads the matching-arch binary, systemd unit, and install.sh,
  then runs install.sh on the target. Equivalent to scripts/deploy.sh
  but for Windows PowerShell.

.PARAMETER User
  SSH username on the target.

.PARAMETER Password
  SSH password for that user. The user must be passwordless-sudo on
  the target so install.sh can run `sudo bash /tmp/install.sh`
  without an interactive prompt. If that isn't the case, run the
  manual scp/ssh steps from the README instead.

.PARAMETER Ip
  IP address of the target.

.PARAMETER Arch
  Target architecture: arm64 (default) or amd64.

.PARAMETER Port
  SSH port on the target (default 22).

.EXAMPLE
  PS> .\scripts\deploy.ps1 -User nvidia -Password secret -Ip 10.0.5.23
  PS> .\scripts\deploy.ps1 -User nvidia -Password secret -Ip 10.0.5.23 -Arch amd64
  PS> $env:SPOTTER_PASS = 'secret'; .\scripts\deploy.ps1 -User nvidia -Password $env:SPOTTER_PASS -Ip 10.0.5.23

.NOTES
  Requires PuTTY's plink.exe + pscp.exe on PATH or in the default
  install location. Install PuTTY from https://putty.org or via
  `choco install putty` / `scoop install putty`.

  Passing the password as a parameter is convenient but stores it in
  the shell history. Prefer setting it via an environment variable.
#>

[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$User,
  [Parameter(Mandatory)][string]$Password,
  [Parameter(Mandatory)][string]$Ip,
  [ValidateSet('arm64','amd64')][string]$Arch = 'arm64',
  [int]$Port = 22
)

$ErrorActionPreference = 'Stop'

# Resolve paths relative to repo root (script lives in scripts/).
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = (Resolve-Path "$ScriptDir\..").Path
$BinDir   = if ($env:BIN_DIR)   { $env:BIN_DIR }   else { Join-Path $Root 'bin' }
$ScriptSrc = if ($env:SCRIPT_SRC) { $env:SCRIPT_SRC } else { Join-Path $Root 'scripts' }

switch ($Arch) {
  'arm64' { $Bin = Join-Path $BinDir 'spotterd-linux-arm64' }
  'amd64' { $Bin = Join-Path $BinDir 'spotterd-linux-x64' }
}

if (-not (Test-Path $Bin)) {
  $suffix = if ($Arch -eq 'amd64') { 'x64' } else { 'arm64' }
  throw "Missing binary $Bin — run 'make agent-linux-$suffix' first."
}

function Find-PuttyTool {
  param([string]$Name)
  $exe = Get-Command $Name -ErrorAction SilentlyContinue
  if ($exe) { return $exe.Source }
  $candidates = @(
    (Join-Path $env:ProgramFiles    "PuTTY\$Name.exe"),
    (Join-Path ${env:ProgramFiles(x86)} "PuTTY\$Name.exe")
  ) | Where-Object { $_ -and (Test-Path $_) }
  if ($candidates) { return $candidates[0] }
  throw "$Name not found. Install PuTTY from https://putty.org (or 'choco install putty')."
}

$plink = Find-PuttyTool 'plink'
$pscp  = Find-PuttyTool 'pscp'

$plinkArgs = @('-batch', '-P', $Port, '-pw', $Password)
$pscpArgs  = @('-batch', '-P', $Port, '-pw', $Password)

function Step { param([string]$Msg) Write-Host "==> $Msg" -ForegroundColor Cyan }
function Ok   { param([string]$Msg) Write-Host " ok] $Msg" -ForegroundColor Green }
function Warn { param([string]$Msg) Write-Host "warn] $Msg" -ForegroundColor Yellow }
function Fail { param([string]$Msg) Write-Host "fail] $Msg" -ForegroundColor Red; throw $Msg }

Step "uploading spotterd ($Arch) -> ${Ip}:$Port"
& $pscp $pscpArgs $Bin "${User}@${Ip}:/tmp/spotterd" | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "pscp spotterd" }

Step "uploading systemd unit"
& $pscp $pscpArgs (Join-Path $ScriptSrc 'spotterd.service') "${User}@${Ip}:/tmp/spotterd.service" | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "pscp unit" }

Step "uploading install.sh"
& $pscp $pscpArgs (Join-Path $ScriptSrc 'install.sh') "${User}@${Ip}:/tmp/install.sh" | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "pscp install.sh" }

Step "running install.sh on $Ip (sudo)"
& $plink $plinkArgs "${User}@${Ip}" 'sudo bash /tmp/install.sh' | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "install.sh" }
Ok "spotterd installed on $Ip"

Step "verifying with /healthz"
$healthz = & $plink $plinkArgs "${User}@${Ip}" 'curl -fsS http://127.0.0.1:9999/healthz' 2>$null
if ($LASTEXITCODE -eq 0 -and $healthz -eq 'ok') {
  Ok "$Ip responds on :9999/healthz"
} else {
  Warn "$Ip did not respond on :9999/healthz — install succeeded but service may need a moment to start."
}
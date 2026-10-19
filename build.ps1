<#
.SYNOPSIS
Builds the HMI and all Go programs in Blue OT into bin\<os>-<arch>\.

.EXAMPLE
.\build.ps1                                    # for this machine
.\build.ps1 -All                               # Windows and Linux (amd64) in one go
.\build.ps1 -OS linux                          # one other target, optionally -Arch arm64
.\build.ps1 -Target linux/arm64,windows/amd64  # any list of GOOS/GOARCH pairs
.\build.ps1 -SkipFrontend                      # reuse the HMI already in scada\backend\internal\web\dist
.\build.ps1 -Test                              # run go test in every module first

Requires Go 1.21+ and, unless -SkipFrontend, Node 18+.
#>
param(
    [string]$OS = '',
    [string]$Arch = '',
    [string[]]$Target = @(),
    [switch]$All,
    [string]$Out = '',
    [switch]$SkipFrontend,
    [switch]$Test
)
$ErrorActionPreference = 'Stop'

$Root = $PSScriptRoot
if (-not $Out) { $Out = Join-Path $Root 'bin' }

# -Target and -All may be combined; -OS/-Arch, or this machine, otherwise
$Targets = @($Target | ForEach-Object { $_ -split ',' } | Where-Object { $_ })
if ($All) { $Targets += 'windows/amd64', 'linux/amd64' }
if ($Targets.Count -eq 0) {
    if (-not $OS) { $OS = (go env GOOS) }
    if (-not $Arch) { $Arch = (go env GOARCH) }
    $Targets = @("$OS/$Arch")
}

# module directory and the commands in it
$Modules = [ordered]@{
    'scada/backend' = @('scada', 'modsim', 'opctl')
    'blue-station'  = @('blue-station')
    'blue-chp'      = @('blue-chp')
    'blue-water'    = @('blue-water')
    'blue-wwtp'     = @('blue-wwtp')
    'blue-proxy'    = @('blue-proxy')
}

# runs a native command in a directory and stops on a non-zero exit code
function Invoke-In([string]$Dir, [scriptblock]$Block) {
    Push-Location (Join-Path $Root $Dir)
    try {
        & $Block
        if ($LASTEXITCODE -ne 0) { throw "misslyckades i ${Dir} (kod $LASTEXITCODE)" }
    } finally { Pop-Location }
}

if (-not $SkipFrontend) {
    Write-Host '==> HMI (scada/frontend)'
    if (-not (Test-Path (Join-Path $Root 'scada/frontend/node_modules'))) {
        Invoke-In 'scada/frontend' { npm ci }
    }
    Invoke-In 'scada/frontend' { npm run build }
}

if ($Test) {
    foreach ($dir in $Modules.Keys) {
        Write-Host "==> go test $dir"
        Invoke-In $dir { go test ./... }
    }
}

$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
try {
    foreach ($t in $Targets) {
        $os, $arch = $t -split '/', 2
        if (-not $os -or -not $arch) { throw "ogiltigt mål: $t (ange GOOS/GOARCH, t.ex. linux/amd64)" }
        $ext = if ($os -eq 'windows') { '.exe' } else { '' }
        $dest = Join-Path $Out "$os-$arch"
        New-Item -ItemType Directory -Force $dest | Out-Null

        $env:GOOS = $os; $env:GOARCH = $arch; $env:CGO_ENABLED = '0'
        foreach ($dir in $Modules.Keys) {
            foreach ($cmd in $Modules[$dir]) {
                Write-Host "==> $cmd ($os/$arch)"
                $exe = Join-Path $dest "$cmd$ext"
                Invoke-In $dir { go build -trimpath -ldflags '-s -w' -o $exe "./cmd/$cmd" }
            }
        }

        # scada needs a data directory; copy the bundled one once, never overwrite edits
        $data = Join-Path $dest 'data'
        if (-not (Test-Path $data)) {
            Copy-Item -Recurse (Join-Path $Root 'scada/backend/data') $data
            Write-Host "==> kopierade scada/backend/data till $data"
        }
        Write-Host "Klart: $dest"
    }
} finally {
    foreach ($k in $saved.Keys) {
        if ($null -eq $saved[$k]) { Remove-Item "env:$k" -ErrorAction SilentlyContinue } else { Set-Item "env:$k" $saved[$k] }
    }
}

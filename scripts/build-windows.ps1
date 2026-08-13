[CmdletBinding()]
param(
    [Parameter()]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$')]
    [string]$Version = 'v1.8.1',

    [Parameter()]
    [ValidateSet('amd64', 'arm64')]
    [string]$Architecture = 'amd64',

    [Parameter()]
    [string]$Output
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$numericMatch = [regex]::Match($Version, '^v(?<major>[0-9]+)\.(?<minor>[0-9]+)\.(?<patch>[0-9]+)')
$resourceVersion = '{0}.{1}.{2}.0' -f $numericMatch.Groups['major'].Value, $numericMatch.Groups['minor'].Value, $numericMatch.Groups['patch'].Value

if (-not $Output) {
    $Output = Join-Path $projectRoot "dist/codedocs_windows_$Architecture.exe"
}

$resolvedOutput = [IO.Path]::GetFullPath($Output)
$outputDirectory = Split-Path -Parent $resolvedOutput
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null

$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED

Push-Location $projectRoot
try {
    & go run github.com/tc-hib/go-winres@v0.3.3 make `
        --in winres/winres.json `
        --arch $Architecture `
        --out cmd/codedocs/rsrc `
        --file-version $resourceVersion `
        --product-version $resourceVersion
    if ($LASTEXITCODE -ne 0) { throw 'Failed to generate Windows resources.' }

    $env:GOOS = 'windows'
    $env:GOARCH = $Architecture
    $env:CGO_ENABLED = '0'
    & go build -trimpath `
        -ldflags "-H=windowsgui -s -w -X codedocs/internal/config.Version=$Version" `
        -o $resolvedOutput ./cmd/codedocs
    if ($LASTEXITCODE -ne 0) { throw 'Failed to build CodeDocs.' }

    $file = Get-Item -LiteralPath $resolvedOutput
    Write-Host "Built $($file.FullName) ($($file.Length) bytes)"
}
finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}

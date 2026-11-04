<#
.SYNOPSIS
    Build dist/drill-linux-bundle.tar.gz - minimal deploy bundle for Linux VM

.DESCRIPTION
    Contents:
      server/deploy-linux/drill-server (SHA must equal the hash on the LAST line of
        server/deploy-linux/drill-server.sha256.txt, abort if mismatch)
      public/
      migrations/
      deploy/linux/*.sh *.service *.yml *.example *.conf Dockerfile
      docs/linux-deployment.md
      docs/vmware-vm-deployment.md
      dist/README-FIRST.md

    EXCLUDES: .env, data/, node_modules, server source, *.bak, *.log, dist/ self

    Flow:
      1) SHA256 verify drill-server, abort if mismatch
      2) Stage temp dir, copy required files
      3) Package tar.gz
      4) List manifest + total size
      5) Verify package contains no .env / no real secrets (just placeholders)

.PARAMETER VerifyOnly
    Run only step 1 (SHA verify against drill-server.sha256.txt) and exit.
#>
param(
    [switch]$VerifyOnly
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

$distDir     = Join-Path $projectRoot "dist"
$bundleDir   = Join-Path $distDir "_bundle"
$bundleTar   = Join-Path $distDir "drill-linux-bundle.tar.gz"
$bundleSha   = Join-Path $distDir "drill-linux-bundle.tar.gz.sha256"

$sigPath     = Join-Path $projectRoot "server\deploy-linux\drill-server.sha256.txt"
$sigLineCount = 9

# ---------------------------------------------------------------
# 1. SHA verify (expected value is read from the signature file)
# ---------------------------------------------------------------
Write-Host "[pack] 1. SHA verify drill-server..."
$elfPath = Join-Path $projectRoot "server\deploy-linux\drill-server"
if (-not (Test-Path $elfPath)) {
    Write-Host "[pack]   FAIL: drill-server not found at $elfPath" -ForegroundColor Red
    exit 1
}
if (-not (Test-Path $sigPath)) {
    Write-Host "[pack]   FAIL: signature file not found at $sigPath" -ForegroundColor Red
    Write-Host "    Please regenerate the signature file first (scripts\build-linux.ps1 -SignOnly)" -ForegroundColor Red
    exit 1
}
$sigLines = @(Get-Content -Path $sigPath -Encoding UTF8)
if ($sigLines.Count -eq 0) {
    Write-Host "[pack]   FAIL: signature file is empty: $sigPath" -ForegroundColor Red
    Write-Host "    Please regenerate the signature file first (scripts\build-linux.ps1 -SignOnly)" -ForegroundColor Red
    exit 1
}
if ($sigLines.Count -ne $sigLineCount) {
    Write-Host "[pack]   FAIL: signature file line count mismatch ($($sigLines.Count) lines, expected $sigLineCount): $sigPath" -ForegroundColor Red
    Write-Host "    Please regenerate the signature file first (scripts\build-linux.ps1 -SignOnly)" -ForegroundColor Red
    exit 1
}
$sigLastLine = $sigLines[$sigLines.Count - 1]
if ($sigLastLine -notmatch '^([0-9A-Fa-f]{64})\s+drill-server\s*$') {
    Write-Host "[pack]   FAIL: last line of signature file has invalid format: '$sigLastLine'" -ForegroundColor Red
    Write-Host "    Expected '<64 hex>  drill-server'" -ForegroundColor Red
    Write-Host "    Please regenerate the signature file first (scripts\build-linux.ps1 -SignOnly)" -ForegroundColor Red
    exit 1
}
$expectedSha = $Matches[1].ToUpper()
$actualSha = (Get-FileHash -Algorithm SHA256 $elfPath).Hash.ToUpper()
if ($actualSha -ne $expectedSha) {
    Write-Host "[pack]   FAIL: SHA mismatch!" -ForegroundColor Red
    Write-Host "    expected: $expectedSha (from $sigPath)"
    Write-Host "    actual  : $actualSha"
    exit 1
}
$elfSizeMB = [math]::Round((Get-Item $elfPath).Length / 1MB, 2)
Write-Host "[pack]   OK ($elfSizeMB MB, $expectedSha)" -ForegroundColor Green

if ($VerifyOnly) {
    Write-Host "[pack]   PASS (-VerifyOnly: skipping staging/package)" -ForegroundColor Green
    exit 0
}

# ---------------------------------------------------------------
# 2. Stage temp dir
# ---------------------------------------------------------------
Write-Host "[pack] 2. staging..."
if (Test-Path $bundleDir) { Remove-Item -Recurse -Force $bundleDir }
New-Item -ItemType Directory -Path $bundleDir | Out-Null

function Copy-ToBundle($src, $dstInBundle) {
    $absSrc = Join-Path $projectRoot $src
    if (-not (Test-Path $absSrc)) {
        Write-Host "[pack]   SKIP (not found): $src" -ForegroundColor DarkYellow
        return
    }
    $dst = Join-Path $bundleDir $dstInBundle
    $dstParent = Split-Path -Parent $dst
    if ($dstParent -and -not (Test-Path $dstParent)) {
        New-Item -ItemType Directory -Force -Path $dstParent | Out-Null
    }
    Copy-Item -Force -Recurse $absSrc $dst
    Write-Host "[pack]   + $src" -ForegroundColor DarkGray
}

$items = @(
    @{ Src = "server\deploy-linux\drill-server"; Dst = "server\deploy-linux\drill-server" },
    @{ Src = "public";                            Dst = "public" },
    @{ Src = "migrations";                        Dst = "migrations" },
    @{ Src = "deploy\linux";                      Dst = "deploy\linux" },
    @{ Src = "docs\linux-deployment.md";          Dst = "docs\linux-deployment.md" },
    @{ Src = "docs\vmware-vm-deployment.md";      Dst = "docs\vmware-vm-deployment.md" },
    @{ Src = "dist\README-FIRST.md";              Dst = "README-FIRST.md" }
)

foreach ($i in $items) { Copy-ToBundle $i.Src $i.Dst }

# ---------------------------------------------------------------
# 3. Safety scan: no .env, no .bak, no stray .sql, no source, no data
# ---------------------------------------------------------------
Write-Host "[pack] 3. safety scan..."
$bad = @()
$all = Get-ChildItem -Recurse -File $bundleDir
foreach ($f in $all) {
    $rel = $f.FullName.Substring($bundleDir.Length + 1).Replace('\','/')
    $name = $f.Name

    if ($name -eq '.env' -or $name -eq '.env.local') { $bad += "FOUND .env: $rel" }
    if ($name -match '\.bak$')                       { $bad += "FOUND .bak: $rel" }
    if ($name -match '\.sql$' -and $rel -notmatch '^migrations/') { $bad += "FOUND stray .sql: $rel" }
    if ($rel -match '^server/internal/' -or $rel -match '^server/cmd/' -or $rel -match '^node_modules/') { $bad += "FOUND source: $rel" }
    if ($rel -match '^data/')                        { $bad += "FOUND data/: $rel" }
}
if ($bad.Count -gt 0) {
    Write-Host "[pack]   FAIL: unsafe items in bundle!" -ForegroundColor Red
    $bad | ForEach-Object { Write-Host "   $_" -ForegroundColor Red }
    exit 1
}
Write-Host "[pack]   OK (no .env / .bak / stray .sql / source / data)" -ForegroundColor Green

# ---------------------------------------------------------------
# 4. Package tar.gz
# ---------------------------------------------------------------
Write-Host "[pack] 4. packaging tar.gz..."
if (Test-Path $bundleTar) { Remove-Item -Force $bundleTar }

Push-Location $bundleDir
try {
    tar -czf $bundleTar .
    if ($LASTEXITCODE -ne 0) {
        throw "tar exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

if (-not (Test-Path $bundleTar)) {
    Write-Host "[pack]   tar failed, falling back to zip..." -ForegroundColor Yellow
    $zip = Join-Path $distDir "drill-linux-bundle.zip"
    if (Test-Path $zip) { Remove-Item -Force $zip }
    Compress-Archive -Path (Join-Path $bundleDir '*') -DestinationPath $zip
    $bundleTar = $zip
}

$bundleSize = [math]::Round((Get-Item $bundleTar).Length / 1MB, 2)
Write-Host "[pack]   bundle size: $bundleSize MB" -ForegroundColor Green

$sha = (Get-FileHash -Algorithm SHA256 $bundleTar).Hash
"$sha  $(Split-Path $bundleTar -Leaf)" | Out-File -FilePath $bundleSha -Encoding ascii
Write-Host "[pack]   SHA256: $sha" -ForegroundColor Green

# ---------------------------------------------------------------
# 5. Manifest (first 40 lines)
# ---------------------------------------------------------------
Write-Host ""
Write-Host "[pack] 5. manifest (first 40 entries)..."
Push-Location $bundleDir
$manifest = @(Get-ChildItem -Recurse | ForEach-Object {
    $rel = $_.FullName.Substring($bundleDir.Length + 1).Replace('\','/')
    if ($_.PSIsContainer) { "$rel/" } else { "$rel ($([math]::Round($_.Length/1KB,1)) KB)" }
})
Pop-Location

$manifest | Select-Object -First 40 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
if ($manifest.Count -gt 40) { Write-Host "  ... total $($manifest.Count) entries" -ForegroundColor DarkGray }

# ---------------------------------------------------------------
# 6. Cleanup staging
# ---------------------------------------------------------------
Write-Host ""
Write-Host "[pack] 6. cleanup staging..."
Remove-Item -Recurse -Force $bundleDir
Write-Host "[pack]   OK" -ForegroundColor Green

Write-Host ""
Write-Host "=============================================" -ForegroundColor Green
Write-Host "  BUNDLE READY" -ForegroundColor Green
Write-Host "  file : $bundleTar" -ForegroundColor Green
Write-Host "  size : $bundleSize MB" -ForegroundColor Green
Write-Host "  SHA  : $sha" -ForegroundColor Green
Write-Host "  items: $($manifest.Count)" -ForegroundColor Green
Write-Host "=============================================" -ForegroundColor Green

# Local Windows build of a Coremon release (same output as build.sh).
# Official releases are built and signed by GitHub Actions on a version tag;
# use this for testing.
#
#   .\build.ps1                     # version from VERSION file
#   .\build.ps1 -Version 1.2.3 -Repo owner/name -Arch amd64
param(
    [string[]]$Arch = @("amd64", "arm64"),
    [string]$Version = "",
    [string]$Repo = ""
)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not $Version) { $Version = (Get-Content VERSION -Raw).Trim() }
$name = "coremon-$Version"
$out = "dist\$name"
if (Test-Path dist) { Remove-Item -Recurse -Force dist }
New-Item -ItemType Directory -Force "$out\release" | Out-Null

function GoBuild($goarch, $pkg, $dst, $ver) {
    $env:GOOS = "linux"; $env:GOARCH = $goarch; $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags "-s -w -X main.version=$ver" -o $dst $pkg
    if ($LASTEXITCODE -ne 0) { throw "build failed: $pkg linux/$goarch" }
}

Write-Host "Running tests..."
Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
go vet ./...; if ($LASTEXITCODE -ne 0) { throw "vet failed" }
go test ./...; if ($LASTEXITCODE -ne 0) { throw "tests failed" }

foreach ($a in $Arch) {
    Write-Host "Building linux/$a ..."
    GoBuild $a ./cmd/coremon-master "$out\release\master\$Version\linux-$a\coremon-master" $Version
    GoBuild $a ./cmd/coremon-agent "$out\release\core\$Version\linux-$a\coremon-agent" $Version
    foreach ($m in Get-ChildItem modules -Directory) {
        $mv = (Get-Content "$($m.FullName)\VERSION" -Raw).Trim()
        GoBuild $a "./modules/$($m.Name)" "$out\release\modules\$($m.Name)\$mv\linux-$a\$($m.Name)" $mv
    }
}

Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
go build -o dist\coremon-sign.exe ./cmd/coremon-sign
if ($LASTEXITCODE -ne 0) { throw "build failed: coremon-sign" }
if (-not $env:COREMON_SIGNING_KEY -and -not (Test-Path keys\signing.key)) {
    Write-Host "Creating local signing key in keys\ ..."
    & dist\coremon-sign.exe keygen -key keys\signing.key -pub keys\signing.pub
}
& dist\coremon-sign.exe release -key keys\signing.key -dir "$out\release"
if ($LASTEXITCODE -ne 0) { throw "signing failed" }

function Subst($src, $dst, $sha) {
    $t = [IO.File]::ReadAllText((Resolve-Path $src).Path) -replace "`r`n", "`n"
    $t = $t.Replace("@REPO@", $Repo).Replace("@VERSION@", $Version).Replace("@SHA256@", $sha)
    [IO.File]::WriteAllText((Join-Path (Get-Location) $dst), $t)
}
foreach ($u in "coremon-master.service", "coremon-master-update.service", "coremon-master-update.path") { Subst "deploy\$u" "$out\$u" "" }
Subst deploy\install-master.sh "$out\install-master.sh" ""

Push-Location dist
tar -czf "$name.tar.gz" $name
Pop-Location
$sha = (Get-FileHash "dist\$name.tar.gz" -Algorithm SHA256).Hash.ToLower()
Subst deploy\install-master.sh dist\install-master.sh $sha
$sha2 = (Get-FileHash dist\install-master.sh -Algorithm SHA256).Hash.ToLower()
[IO.File]::WriteAllText((Join-Path (Get-Location) "dist\SHA256SUMS"), "$sha  $name.tar.gz`n$sha2  install-master.sh`n")
Write-Host ""
Write-Host "Release ready: dist\$name.tar.gz (+ dist\install-master.sh, dist\SHA256SUMS)"

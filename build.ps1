param([switch]$Dev, [switch]$CompileDev)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
if (-not $Dev) {
    $bin = Join-Path $PSScriptRoot 'dist'
    $running = Get-Process -Name 'ShellWorkflow*','ThunderShip*' -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and (Split-Path -Parent $_.Path) -eq $bin }
    if ($running) { throw '请先保存草稿并退出霆舟，再重新构建；正式程序固定为 dist/ThunderShip.exe。' }
}
$goVersion = [regex]::Match((Get-Content go.mod -Raw), '(?m)^go\s+(\d+\.\d+)').Groups[1].Value
$sdkRoot = Join-Path $env:USERPROFILE 'sdk'
$sdk = Get-ChildItem -LiteralPath $sdkRoot -Directory -Filter "go$goVersion.*" |
    Where-Object { Test-Path (Join-Path $_.FullName 'bin\go.exe') } |
    Sort-Object { [version]($_.Name -replace '^go', '') } -Descending | Select-Object -First 1
if (-not $sdk) { throw "Install a Go $goVersion SDK under $sdkRoot first." }
$env:PATH = (Join-Path $sdk.FullName 'bin') + ';' + $env:PATH
$wails = Join-Path (& go env GOPATH) 'bin\wails3.exe'
if (-not (Test-Path $wails)) { throw 'Install the pinned CLI: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0' }
if (-not (Test-Path frontend\node_modules)) {
    Push-Location frontend
    try {
        & npm.cmd ci --os=win32 --cpu=x64 --include=optional --no-audit
        if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
    } finally { Pop-Location }
}
$cliMetadata = & go version -m $wails | Out-String
if ($cliMetadata -notmatch 'github.com/wailsapp/wails/v3\s+v3\.0\.0\s') { throw 'This project requires wails3 v3.0.0.' }
$env:PATH = (Split-Path -Parent $wails) + ';' + $env:PATH
if ($Dev) {
    & $wails dev -config ./build/config.yml -port 34115
    if ($LASTEXITCODE -ne 0) { throw 'Wails development server failed.' }
    exit
}
& $wails generate bindings -ts -i
if ($LASTEXITCODE -ne 0) { throw 'Binding generation failed.' }
Push-Location frontend
try {
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
} finally { Pop-Location }
$resource = Join-Path $PSScriptRoot 'wails_windows_amd64.syso'
try {
    & $wails generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out $resource
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
    if ($CompileDev) {
        & go build -buildvcs=false -o dist/ThunderShip.exe .
    } else {
        & go build -tags production -trimpath -buildvcs=false -ldflags '-s -w -H windowsgui' -o dist/ThunderShip.exe .
    }
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
} finally {
    if (Test-Path -LiteralPath $resource) { Remove-Item -LiteralPath $resource }
}

# Native Windows development. Tools are portable and PATH changes are process-local.
[CmdletBinding()]
param(
    [ValidateSet('Setup', 'Build', 'Run', 'Client', 'Test', 'Env')]
    [string]$Action = 'Run',
    [string]$ToolsDir = (Join-Path $env:LOCALAPPDATA 'KoitoDev'),
    [int]$Port = 4110
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$localDir = Join-Path $repoRoot '.local'
$sdkRoot = Join-Path $ToolsDir 'vips-dev-8.16'

function Assert-Exit([string]$Operation) {
    if ($LASTEXITCODE -ne 0) { throw "$Operation failed (exit $LASTEXITCODE)." }
}

function Get-ToolArchive([string]$Name, [string]$Url, [string]$Sha256) {
    $archive = Join-Path $ToolsDir $Name
    if (-not (Test-Path -LiteralPath $archive)) {
        & curl.exe --fail --location --retry 2 --max-time 300 $Url --output $archive
        Assert-Exit "Download $Name"
    }
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $Sha256) {
        throw "Checksum mismatch for $archive. The archive was not extracted."
    }
    return $archive
}

function Initialize-NativeSdk {
    $compatDir = Join-Path $ToolsDir 'bimg-compat/include/vips'
    New-Item -ItemType Directory -Force -Path $compatDir | Out-Null
    # The Windows SDK omits deprecated vips7compat.h. This is the only name
    # bimg uses from it; upstream defines exactly this alias as well:
    # https://github.com/libvips/libvips/blob/v8.14.5/libvips/include/vips/vips7compat.h
    @'
#ifndef KOITO_BIMG_VIPS7COMPAT_H
#define KOITO_BIMG_VIPS7COMPAT_H
#include <vips/vips.h>
#define im__print_all vips_object_print_all
#endif
'@ | Set-Content -LiteralPath (Join-Path $compatDir 'vips7compat.h') -Encoding Ascii

    # Use relocatable, local copies. The SDK's glib.pc advertises a separate
    # libintl even though this distribution bundles it and ships no such library.
    $pcDir = Join-Path $ToolsDir 'vips-pkgconfig'
    New-Item -ItemType Directory -Force -Path $pcDir | Out-Null
    foreach ($folder in @('lib/pkgconfig', 'share/pkgconfig')) {
        if (-not (Test-Path -LiteralPath (Join-Path $sdkRoot $folder))) { continue }
        Get-ChildItem -LiteralPath (Join-Path $sdkRoot $folder) -Filter '*.pc' | ForEach-Object {
            $content = Get-Content -LiteralPath $_.FullName -Raw
            $prefix = 'prefix=' + $sdkRoot.Replace('\', '/')
            $content = [regex]::Replace($content, '(?m)^prefix=.*$', [System.Text.RegularExpressions.MatchEvaluator]{ param($match) $prefix })
            if ($_.Name -eq 'glib-2.0.pc') { $content = $content.Replace(' -lintl', '') }
            [IO.File]::WriteAllText((Join-Path $pcDir $_.Name), $content)
        }
    }
    $env:PATH = "$(Join-Path $ToolsDir 'go/bin');$(Join-Path $ToolsDir 'w64devkit/bin');$(Join-Path $sdkRoot 'bin');$env:PATH"
    $env:PKG_CONFIG_PATH = $pcDir
    $env:CGO_ENABLED = '1'
    $env:CC = 'gcc'
    $include = (Join-Path $ToolsDir 'bimg-compat/include').Replace('\', '/')
    $env:CGO_CFLAGS = '"-I' + $include + '" -DKOITO_NATIVE_SDK=1'
}

if ($Action -eq 'Setup') {
    if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') { throw 'This setup currently targets x64 Windows.' }
    New-Item -ItemType Directory -Force -Path $ToolsDir | Out-Null
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $goZip = Get-ToolArchive 'go-direct.zip' 'https://dl.google.com/go/go1.25.0.windows-amd64.zip' '89EFB4F9B30812EEE083CC1770FDD2913C14D301064F6454851428F9707D190B'
    if (-not (Test-Path -LiteralPath (Join-Path $ToolsDir 'go/bin/go.exe'))) {
        [IO.Compression.ZipFile]::ExtractToDirectory($goZip, $ToolsDir)
    }
    $compiler = Get-ToolArchive 'w64devkit-x64-2.11.0.7z.exe' 'https://github.com/skeeto/w64devkit/releases/download/v2.11.0/w64devkit-x64-2.11.0.7z.exe' '54C2BB9962E8A30C8270E34C5EFC3D5FEE579F53087FA7D435B95FBBA2AF7B2B'
    if (-not (Test-Path -LiteralPath (Join-Path $ToolsDir 'w64devkit/bin/gcc.exe'))) {
        $extract = Start-Process -FilePath $compiler -ArgumentList @('-y', ('-o"{0}"' -f $ToolsDir)) -WindowStyle Hidden -Wait -PassThru
        if ($extract.ExitCode -ne 0) { throw 'Compiler extraction failed.' }
    }
    $vipsZip = Get-ToolArchive 'vips-dev-w64-web-8.16.1.zip' 'https://github.com/libvips/build-win64-mxe/releases/download/v8.16.1/vips-dev-w64-web-8.16.1.zip' 'CD9166D58329C80262AC0ED602F4F2954E39764010075BFC39E3C814308D1483'
    if (-not (Test-Path -LiteralPath (Join-Path $sdkRoot 'bin/vips.exe'))) {
        [IO.Compression.ZipFile]::ExtractToDirectory($vipsZip, $ToolsDir)
    }
}

foreach ($required in @('go/bin/go.exe', 'w64devkit/bin/gcc.exe', 'vips-dev-8.16/bin/vips.exe')) {
    if (-not (Test-Path -LiteralPath (Join-Path $ToolsDir $required))) {
        throw "Missing $required. Run: .\scripts\windows.ps1 -Action Setup"
    }
}
Initialize-NativeSdk

function Build-Client {
    Push-Location (Join-Path $repoRoot 'client')
    try {
        & corepack.cmd yarn install --immutable; Assert-Exit 'Client dependency installation'
        & corepack.cmd yarn build; Assert-Exit 'Client build'
    } finally { Pop-Location }
}
function Build-Backend {
    New-Item -ItemType Directory -Force -Path (Join-Path $localDir 'bin') | Out-Null
    & go.exe build -o (Join-Path $localDir 'bin/koito.exe') ./cmd/api
    Assert-Exit 'Backend build'
}

Push-Location $repoRoot
try {
    switch ($Action) {
        'Setup' { & go.exe version; & vips.exe --version; Write-Host 'Native tools ready. Next: .\scripts\windows.ps1 -Action Run' }
        'Env' { & go.exe version; & gcc.exe --version | Select-Object -First 1; & vips.exe --version }
        'Build' { Build-Client; Build-Backend }
        'Test' {
            $packages = & go.exe list ./... | Where-Object { $_ -notmatch '/node_modules/' }
            Assert-Exit 'List Go packages'
            & go.exe test $packages; Assert-Exit 'Go tests'
            Push-Location client
            try {
                & corepack.cmd yarn install --immutable; Assert-Exit 'Client dependency installation'
                & corepack.cmd yarn typecheck; Assert-Exit 'Client type check'
            } finally { Pop-Location }
        }
        'Client' {
            $env:KOITO_DEV_API = "http://127.0.0.1:$Port"
            Push-Location client
            try {
                & corepack.cmd yarn install --immutable; Assert-Exit 'Client dependency installation'
                & corepack.cmd yarn dev
                Assert-Exit 'Client development server'
            } finally { Pop-Location }
        }
        'Run' {
            Build-Client; Build-Backend
            $loginFile = Join-Path $localDir 'login.json'
            if (-not (Test-Path -LiteralPath $loginFile)) {
                @{username='developer'; password=[guid]::NewGuid().ToString('N')} | ConvertTo-Json | Set-Content -LiteralPath $loginFile
            }
            $login = Get-Content -LiteralPath $loginFile -Raw | ConvertFrom-Json
            $env:KOITO_CONFIG_DIR = Join-Path $localDir 'state'
            $env:KOITO_BIND_ADDR = '127.0.0.1'
            $env:KOITO_LISTEN_PORT = [string]$Port
            $env:KOITO_DEFAULT_USERNAME = $login.username
            $env:KOITO_DEFAULT_PASSWORD = $login.password
            $env:KOITO_SKIP_IMPORT = 'true'
            $env:KOITO_DISABLE_MUSICBRAINZ = 'true'
            $env:KOITO_DISABLE_DEEZER = 'true'
            $env:KOITO_DISABLE_COVER_ART_ARCHIVE = 'true'
            $env:KOITO_LOG_LEVEL = 'warn'
            Write-Host "Local Koito: http://127.0.0.1:$Port/imports"
            Write-Host "Local login credentials: $loginFile"
            Write-Host 'Press Ctrl+C to stop. State is preserved under .local/state.'
            & (Join-Path $localDir 'bin/koito.exe')
            Assert-Exit 'Koito'
        }
    }
} finally { Pop-Location }

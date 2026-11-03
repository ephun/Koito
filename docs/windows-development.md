# Local Windows development

Develop and test on this PC; deploy to the server separately. The native setup
uses the real libvips image library, with no test overlay or replacement image
processor. Docker is optional.

From PowerShell in the repository:

```powershell
.\scripts\windows.ps1 -Action Setup  # once, or after changing tool versions
.\scripts\windows.ps1 -Action Run
```

Open **http://127.0.0.1:4110/imports**. The first run creates a local account; its
username and generated password are in `.local/login.json`. This file and the
local database are excluded from Git. The server binds to localhost, uses
`.local/state` for its database/source files, and skips automatic startup imports.
It never connects to or changes the production server. Existing `data_dump` files
are not automatically imported. Stop with Ctrl+C; local state persists.

For frontend hot reload, keep the backend running and use another PowerShell:

```powershell
.\scripts\windows.ps1 -Action Client
```

Use the Vite URL printed in that terminal (normally http://localhost:5173).
Vite forwards API requests to the backend on port 4110. For a different backend
port, pass the same `-Port` to both the Run and Client commands.

Other commands:

```powershell
.\scripts\windows.ps1 -Action Build # client and native backend
.\scripts\windows.ps1 -Action Test  # Go suite and client type checking
. .\scripts\windows.ps1 -Action Env # configure this terminal for go commands
go test ./internal/sources
```

Node.js and Corepack are prerequisites for frontend commands. Setup downloads
checksum-pinned archives for Go 1.25.0, w64devkit 2.11.0, and the libvips 8.16.1
Windows web SDK. The tools live in `%LOCALAPPDATA%\KoitoDev`; pass `-ToolsDir`
consistently to use another location. No administrator access, permanent PATH
changes, WSL, or Docker is needed.

## SDK compatibility

Koito's existing bimg binding includes a deprecated header omitted from official
Windows SDKs. Setup supplies only its required debug alias:
`im__print_all` maps to `vips_object_print_all`, exactly as in the
[upstream compatibility header](https://github.com/libvips/libvips/blob/v8.14.5/libvips/include/vips/vips7compat.h).
Image operations continue calling the real libvips DLLs.

Setup also creates local copies of the SDK's pkg-config metadata with relocated
prefixes and removes its stale `-lintl` entry from glib's public link flags: the
Windows distribution bundles this dependency and ships no separate intl library.
The downloaded SDK and Go module cache are not modified. The pinned SDK retains
the vector API required by this version of bimg; newer SDKs are not interchangeable
without checking the binding.

`imagecache/TestCompressImage` verifies real PNG-to-WebP conversion at all Koito
image sizes, including source-size output. Engine integration tests also exercise
image uploads and disk caching. Linux builds retain their existing dependency
setup; these SDK adjustments apply only to the Windows development command.

$ErrorActionPreference = 'Stop'

# Find signtool.exe from Windows SDK
$signtool = Get-ChildItem -Path "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Recurse -Filter signtool.exe |
    Where-Object { $_.FullName -match 'x64' } |
    Sort-Object FullName -Descending |
    Select-Object -First 1

if (-not $signtool) {
    Write-Error "signtool.exe not found"
    exit 1
}

Write-Host "Using signtool: $($signtool.FullName)"

& $signtool.FullName sign /fd SHA256 /a /f ./dist/test.pfx /p test123 ./dist/foo.msix

if ($LASTEXITCODE -ne 0) {
    Write-Error "signtool sign failed with exit code $LASTEXITCODE"
    exit 1
}

Write-Host "MSIX package signed successfully"

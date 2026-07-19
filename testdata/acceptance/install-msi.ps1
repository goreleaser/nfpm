$ErrorActionPreference = 'Stop'

$msi = Resolve-Path ./dist/foo.msi
$log = "./dist/install.log"

# Marker files written by the nfpm maintainer scripts (run as SYSTEM, whose
# TEMP resolves under the Windows directory).
$markers = @{
    preinstall  = Join-Path $env:SystemRoot 'Temp\nfpm-acc-preinstall.txt'
    postinstall = Join-Path $env:SystemRoot 'Temp\nfpm-acc-postinstall.txt'
    preremove   = Join-Path $env:SystemRoot 'Temp\nfpm-acc-preremove.txt'
    postremove  = Join-Path $env:SystemRoot 'Temp\nfpm-acc-postremove.txt'
}
$markers.Values | ForEach-Object { Remove-Item -Force $_ -ErrorAction SilentlyContinue }

Write-Host "Installing MSI: $msi ($((Get-Item $msi).Length) bytes)"

$proc = Start-Process msiexec.exe -ArgumentList "/i `"$msi`" /qn /norestart /l*v `"$log`"" -Wait -PassThru
if ($proc.ExitCode -ne 0) {
    Write-Host "msiexec install failed with exit code $($proc.ExitCode)"
    if (Test-Path $log) { Get-Content $log | Write-Host }
    exit 1
}
Write-Host "Package installed successfully"

foreach ($hook in @('preinstall', 'postinstall')) {
    if (-not (Test-Path $markers[$hook])) {
        Write-Error "$hook script did not run (marker $($markers[$hook]) missing)"
        exit 1
    }
}
Write-Host "Install scripts ran correctly"

# Verify the installed executable runs and prints the expected sentinel.
$exe = "C:\Program Files\NfpmMsiTest\testapp.exe"
if (-not (Test-Path $exe)) {
    Write-Error "Installed executable not found at $exe"
    exit 1
}

$output = & $exe 2>&1
if ($output -ne "nfpm-msix-test-ok") {
    Write-Error "Expected 'nfpm-msix-test-ok' but got '$output'"
    exit 1
}
Write-Host "Installed application ran correctly"

# Uninstall.
$proc = Start-Process msiexec.exe -ArgumentList "/x `"$msi`" /qn /norestart" -Wait -PassThru
if ($proc.ExitCode -ne 0) {
    Write-Error "msiexec uninstall failed with exit code $($proc.ExitCode)"
    exit 1
}
Write-Host "Package uninstalled successfully"

foreach ($hook in @('preremove', 'postremove')) {
    if (-not (Test-Path $markers[$hook])) {
        Write-Error "$hook script did not run (marker $($markers[$hook]) missing)"
        exit 1
    }
}
Write-Host "Remove scripts ran correctly"

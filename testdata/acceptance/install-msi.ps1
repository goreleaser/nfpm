$ErrorActionPreference = 'Stop'

$msi = Resolve-Path ./dist/foo.msi
$log = "./dist/install.log"

Write-Host "Installing MSI: $msi ($((Get-Item $msi).Length) bytes)"

$proc = Start-Process msiexec.exe -ArgumentList "/i `"$msi`" /qn /norestart /l*v `"$log`"" -Wait -PassThru
if ($proc.ExitCode -ne 0) {
    Write-Host "msiexec install failed with exit code $($proc.ExitCode)"
    if (Test-Path $log) { Get-Content $log | Write-Host }
    exit 1
}
Write-Host "Package installed successfully"

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

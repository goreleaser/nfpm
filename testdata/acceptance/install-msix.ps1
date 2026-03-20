$ErrorActionPreference = 'Stop'

# Check developer mode status
$devMode = Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" -ErrorAction SilentlyContinue
Write-Host "Developer mode: AllowDevelopmentWithoutDevLicense = $($devMode.AllowDevelopmentWithoutDevLicense)"
Write-Host "Sideloading: AllowAllTrustedApps = $($devMode.AllowAllTrustedApps)"

# Show package info before install
Write-Host "Package path: ./dist/foo.msix"
Write-Host "Package size: $((Get-Item ./dist/foo.msix).Length) bytes"

try {
    Add-AppxPackage -Path ./dist/foo.msix -Verbose
    Write-Host "Package installed successfully"
} catch {
    Write-Host "Install failed: $_"
    Write-Host "Exception: $($_.Exception.Message)"
    if ($_.Exception.InnerException) {
        Write-Host "Inner: $($_.Exception.InnerException.Message)"
    }

    # Try to get the event log for more details
    $activityId = $_.Exception.Message -match '\[ActivityId\]\s*([a-f0-9-]+)' | Out-Null
    if ($Matches) {
        Write-Host "ActivityId: $($Matches[1])"
        Get-AppPackageLog -ActivityID $Matches[1] | Write-Host
    }

    exit 1
}

# Verify installation
$pkg = Get-AppxPackage -Name "com.example.foo"
if ($pkg) {
    Write-Host "Verified: $($pkg.PackageFullName)"
    Write-Host "InstallLocation: $($pkg.InstallLocation)"
} else {
    Write-Error "Package com.example.foo not found after installation"
    exit 1
}

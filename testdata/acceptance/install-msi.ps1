param(
    # The package built from msi.install.yaml (or msi.signed.yaml): installed,
    # exercised, and removed.
    [string]$Msi = './dist/foo.msi',
    # The same product one version up (msi.upgrade.yaml). When given, it is
    # installed over $Msi and must replace it in place: exactly one entry in
    # Apps & Features, the new version, the old files gone with the product.
    [string]$Upgrade,
    # A package whose postinstall script fails (msi.rollback.yaml). When given,
    # installing it must fail and leave nothing behind.
    [string]$Rollback
)
$ErrorActionPreference = 'Stop'

$msi = Resolve-Path $Msi
$installDir = 'C:\Program Files\NfpmMsiTest'
$exe = Join-Path $installDir 'testapp.exe'
$svcExe = Join-Path $installDir 'testsvc.exe'
$conf = Join-Path $installDir 'etc\testapp\testapp.conf'
$shortcut = Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\Nfpm MSI Test.lnk'
$regKey = 'HKLM:\Software\NfpmMsiTest'
$serviceName = 'NfpmMsiTestSvc'

# Marker files written by the nfpm maintainer scripts (run as SYSTEM, whose
# TEMP resolves under the Windows directory).
$markers = @{
    preinstall  = Join-Path $env:SystemRoot 'Temp\nfpm-acc-preinstall.txt'
    postinstall = Join-Path $env:SystemRoot 'Temp\nfpm-acc-postinstall.txt'
    preremove   = Join-Path $env:SystemRoot 'Temp\nfpm-acc-preremove.txt'
    postremove  = Join-Path $env:SystemRoot 'Temp\nfpm-acc-postremove.txt'
}

function Reset-Markers {
    $markers.Values | ForEach-Object { Remove-Item -Force $_ -ErrorAction SilentlyContinue }
}

function Assert-Markers([string[]]$hooks) {
    foreach ($hook in $hooks) {
        if (-not (Test-Path $markers[$hook])) {
            throw "$hook script did not run (marker $($markers[$hook]) missing)"
        }
    }
}

function Assert-NoMarkers([string[]]$hooks) {
    foreach ($hook in $hooks) {
        if (Test-Path $markers[$hook]) {
            throw "$hook script ran but must not have (marker $($markers[$hook]) present)"
        }
    }
}

function Invoke-Msiexec([string[]]$arguments, [string]$log) {
    $proc = Start-Process msiexec.exe -ArgumentList ($arguments + @('/qn', '/norestart', '/l*v', "`"$log`"")) -Wait -PassThru
    return $proc.ExitCode
}

function Show-Log([string]$log) {
    if (Test-Path $log) { Get-Content $log | Write-Host }
}

# Installed products with the given display name, from the Apps & Features
# registry (what msiexec /x and Windows Installer's Upgrade table consult).
function Get-InstalledProducts([string]$displayName) {
    return @(Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -eq $displayName })
}

function Assert-Installed([string]$version) {
    foreach ($path in @($exe, $svcExe, $conf, $shortcut)) {
        if (-not (Test-Path $path)) { throw "expected $path after install" }
    }
    Write-Host "Files, shortcut and service executable are in place"

    $output = & $exe 2>&1
    if ($output -ne 'nfpm-msix-test-ok') {
        throw "expected the installed app to print 'nfpm-msix-test-ok' but got '$output'"
    }
    Write-Host "Installed application ran correctly"

    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if (-not $svc) { throw "service $serviceName is not registered" }
    if ($svc.Status -ne 'Stopped') { throw "service $serviceName should be registered but not started, status is $($svc.Status)" }
    Write-Host "Service $serviceName is registered"

    $installPath = (Get-ItemProperty $regKey -ErrorAction SilentlyContinue).InstallPath
    if ($installPath -ne "$installDir\") {
        throw "expected registry value InstallPath '$installDir\' but got '$installPath'"
    }
    Write-Host "Registry value is in place"

    $products = Get-InstalledProducts 'NfpmMsiTest'
    if ($products.Count -ne 1) { throw "expected exactly one NfpmMsiTest product, found $($products.Count)" }
    if ($products[0].DisplayVersion -ne $version) {
        throw "expected product version $version but found $($products[0].DisplayVersion)"
    }
    Write-Host "Exactly one NfpmMsiTest product is registered, version $version"
}

function Assert-Removed {
    if (Test-Path $installDir) { throw "install directory $installDir still exists after uninstall" }
    if (Test-Path $shortcut) { throw "shortcut $shortcut still exists after uninstall" }
    if (Test-Path $regKey) { throw "registry key $regKey still exists after uninstall" }
    if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) { throw "service $serviceName still exists after uninstall" }
    if ((Get-InstalledProducts 'NfpmMsiTest').Count -ne 0) { throw "NfpmMsiTest is still registered after uninstall" }
    Write-Host "Files, shortcut, registry key, service and product registration are gone"
}

# --- install -----------------------------------------------------------------

Reset-Markers
Write-Host "Installing MSI: $msi ($((Get-Item $msi).Length) bytes)"
$code = Invoke-Msiexec @('/i', "`"$msi`"") './dist/install.log'
if ($code -ne 0) {
    Write-Host "msiexec install failed with exit code $code"
    Show-Log './dist/install.log'
    exit 1
}
Write-Host "Package installed successfully"
Assert-Markers @('preinstall', 'postinstall')
Assert-NoMarkers @('preremove', 'postremove')
Write-Host "Install scripts ran correctly"
Assert-Installed '1.0.0'

# --- upgrade in place --------------------------------------------------------

if ($Upgrade) {
    $upgrade = Resolve-Path $Upgrade
    Reset-Markers
    Write-Host "Upgrading with MSI: $upgrade"
    $code = Invoke-Msiexec @('/i', "`"$upgrade`"") './dist/upgrade.log'
    if ($code -ne 0) {
        Write-Host "msiexec upgrade failed with exit code $code"
        Show-Log './dist/upgrade.log'
        exit 1
    }
    Write-Host "Package upgraded successfully"
    # The new product's install hooks run; the old product is replaced, not
    # uninstalled, so its remove hooks stay quiet.
    Assert-Markers @('preinstall', 'postinstall')
    Assert-NoMarkers @('preremove', 'postremove')
    Write-Host "Upgrade scripts ran correctly"
    Assert-Installed '1.0.1'
    $msi = $upgrade
}

# --- uninstall ---------------------------------------------------------------

Reset-Markers
$code = Invoke-Msiexec @('/x', "`"$msi`"") './dist/uninstall.log'
if ($code -ne 0) {
    Write-Host "msiexec uninstall failed with exit code $code"
    Show-Log './dist/uninstall.log'
    exit 1
}
Write-Host "Package uninstalled successfully"
Assert-Markers @('preremove', 'postremove')
Write-Host "Remove scripts ran correctly"
Assert-Removed

# --- failed postinstall rolls back -------------------------------------------

if ($Rollback) {
    $rollback = Resolve-Path $Rollback
    Reset-Markers
    Write-Host "Installing MSI with a failing postinstall: $rollback"
    $code = Invoke-Msiexec @('/i', "`"$rollback`"") './dist/rollback.log'
    if ($code -eq 0) {
        Write-Host "msiexec reported success although the postinstall script failed"
        Show-Log './dist/rollback.log'
        exit 1
    }
    Write-Host "msiexec failed as expected with exit code $code"
    Assert-Markers @('preinstall')
    if (Test-Path 'C:\Program Files\NfpmMsiRollback') { throw "install directory of the rolled-back product still exists" }
    if ((Get-InstalledProducts 'NfpmMsiRollback').Count -ne 0) { throw "the rolled-back product is registered" }
    Write-Host "Failed install rolled back cleanly"
}

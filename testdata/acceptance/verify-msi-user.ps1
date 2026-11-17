param([Parameter(Mandatory)][string]$Fixture)
$ErrorActionPreference = 'Stop'
Start-Transcript -Path (Join-Path $Fixture 'user-proof.log') -Force
try {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'User process is elevated' }
    if (@($identity.Groups.Value) -contains 'S-1-5-32-544') { throw 'User belongs to Administrators' }
    if ((Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System').EnableLUA -ne 1) { throw 'UAC must be enabled' }
    foreach ($root in @('HKLM:', 'HKCU:')) {
        $policy = Get-ItemProperty "$root\Software\Policies\Microsoft\Windows\Installer" -ErrorAction SilentlyContinue
        if ($policy.AlwaysInstallElevated -eq 1) { throw 'AlwaysInstallElevated would invalidate proof' }
    }
    # CreateProcessWithLogonW inherits the parent's environment. Resolve the
    # loaded profile by the actual token SID before launching the installer.
    $profile = (Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\$($identity.User.Value)").ProfileImagePath
    $profile = [Environment]::ExpandEnvironmentVariables($profile)
    if (-not (Test-Path $profile)) { throw 'Standard-user profile was not loaded' }
    $env:PSModulePath = "$profile\Documents\WindowsPowerShell\Modules;" + [Environment]::GetEnvironmentVariable('PSModulePath', 'Machine') + ";$PSHOME\Modules"
    $env:USERPROFILE = $profile
    $env:LOCALAPPDATA = Join-Path $profile 'AppData\Local'
    $env:APPDATA = Join-Path $profile 'AppData\Roaming'
    $env:TEMP = Join-Path $env:LOCALAPPDATA 'Temp'
    $env:TMP = $env:TEMP
    if ((Get-ItemProperty 'HKLM:\Software\Policies\Microsoft\Windows\Installer').DisableMSI -ne 0) { throw 'Runner must permit ordinary unmanaged per-user installs' }
    Write-Host "STANDARD_USER_TOKEN sid=$($identity.User.Value) elevated=False administratorMember=False"
    $installer = New-Object -ComObject WindowsInstaller.Installer
    $cases = Get-Content (Join-Path $Fixture 'user-cases.json') -Encoding UTF8 -Raw | ConvertFrom-Json
    foreach ($case in $cases) {
        $package = Join-Path $Fixture "foo-user-$($case.name).msi"
        $summary = $installer.SummaryInformation($package, 0)
        if ([int]$summary.Property(15) -ne 10) { throw 'Per-user summary must have WordCount 2|8' }
        if ([int]$summary.Property(14) -lt 400) { throw 'Per-user package must require MSI 4.0 or newer' }
        if ($summary.Property(3) -cne $case.product) { throw 'Native summary changed the product name' }
        if ($summary.Property(4) -cne $case.manufacturer) { throw 'Native summary changed the manufacturer name' }
        $database = $installer.OpenDatabase($package, 0)
        $view = $database.OpenView('SELECT `Value` FROM `Property` WHERE `Property` = ''UpgradeCode''')
        $view.Execute()
        $upgradeCode = $view.Fetch().StringData(1)
        $view.Close()
        $target = Join-Path $env:LOCALAPPDATA "Programs\NfpmMsiUser$($case.name)"
        $marker = Join-Path $env:LOCALAPPDATA 'nfpm-msi-user-action.txt'
        Remove-Item $marker -Force -ErrorAction SilentlyContinue
        $installLog = Join-Path $Fixture "user-$($case.name)-install.log"
        $process = Start-Process msiexec.exe -ArgumentList "/i `"$package`" /qn /norestart /L*V `"$installLog`"" -Wait -PassThru
        if ($process.ExitCode -ne 0) { throw "Standard-user install failed: $($process.ExitCode)" }
        $products = @($installer.RelatedProducts($upgradeCode))
        if ($products.Count -ne 1) { throw 'Expected exactly one per-user product' }
        if ($installer.ProductInfo($products[0], 'ProductName') -cne $case.product) { throw 'Installed product name changed' }
        if ($installer.ProductInfo($products[0], 'Publisher') -cne $case.manufacturer) { throw 'Installed manufacturer name changed' }
        $executable = Join-Path $target 'testapp.exe'
        # Get-FileHash is not available on every Windows PowerShell installation.
        if ([Convert]::ToBase64String([IO.File]::ReadAllBytes($executable)) -cne [Convert]::ToBase64String([IO.File]::ReadAllBytes((Join-Path $Fixture 'testapp.exe')))) { throw 'Installed executable differs' }
        if ((& $executable).Trim() -ne 'nfpm-msix-test-ok') { throw 'Installed executable did not run' }
        if ((Get-Content $marker -Raw).Trim() -ne $identity.User.Value) { throw 'Deferred action did not run as the standard user' }
        $key = "HKCU:\Software\NfpmMsiUser$($case.name)"
        if ((Get-ItemProperty $key).Installed -ne '1') { throw 'HKCU registry resource missing' }
        $removeLog = Join-Path $Fixture "user-$($case.name)-remove.log"
        $process = Start-Process msiexec.exe -ArgumentList "/x `"$package`" /qn /norestart /L*V `"$removeLog`"" -Wait -PassThru
        if ($process.ExitCode -ne 0) { throw "Standard-user uninstall failed: $($process.ExitCode)" }
        if (@($installer.RelatedProducts($upgradeCode)).Count -ne 0) { throw 'Per-user product remains registered' }
        if (Test-Path $executable) { throw 'Per-user executable remains installed' }
        if (Get-ItemProperty $key -Name Installed -ErrorAction SilentlyContinue) { throw 'HKCU registry resource remains' }
        Remove-Item $marker -Force
        Write-Host "STANDARD_USER_UNICODE_INSTALL_REMOVE_OK case=$($case.name) sid=$($identity.User.Value)"
    }
} finally {
    Stop-Transcript
}

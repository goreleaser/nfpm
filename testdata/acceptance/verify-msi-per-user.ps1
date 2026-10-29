$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows') { throw 'Standard-account provisioning requires a disposable GitHub Windows runner' }
if ((Get-CimInstance Win32_ComputerSystem).PartOfDomain) { throw 'Do not change domain-managed installer policy' }
$policyPath = 'HKLM:\Software\Policies\Microsoft\Windows\Installer'
$policyKeyCreated = -not (Test-Path $policyPath)
$policyChanged = $false
$createdUser = $false
$userName = 'nfpm' + [guid]::NewGuid().ToString('N').Substring(0, 10)
$fixture = Join-Path $env:PUBLIC ('nfpm-msi-user-' + [guid]::NewGuid())
try {
    # Windows Server normally blocks unmanaged, non-elevated MSI installs.
    # Permit them only for this test, without enabling AlwaysInstallElevated.
    if ($policyKeyCreated) { New-Item $policyPath -Force | Out-Null }
    $policyKey = Get-Item $policyPath
    $hadDisableMSI = $policyKey.GetValueNames() -contains 'DisableMSI'
    if ($hadDisableMSI) {
        $oldDisableMSI = $policyKey.GetValue('DisableMSI')
        $oldDisableMSIKind = $policyKey.GetValueKind('DisableMSI')
    }
    New-ItemProperty $policyPath -Name DisableMSI -PropertyType DWord -Value 0 -Force | Out-Null
    $policyChanged = $true
    New-Item -ItemType Directory $fixture | Out-Null
    Copy-Item './dist/foo-user-*.msi', './dist/testapp.exe', "$PSScriptRoot\verify-msi-user.ps1", "$PSScriptRoot\user-cases.json" $fixture
    $password = ConvertTo-SecureString ('Nfpm!9a' + [guid]::NewGuid().ToString('N')) -AsPlainText -Force
    New-LocalUser -Name $userName -Password $password -AccountNeverExpires | Out-Null
    $createdUser = $true
    $user = Get-LocalUser $userName
    if (@(Get-LocalGroupMember -SID 'S-1-5-32-545').SID.Value -notcontains $user.SID.Value) { Add-LocalGroupMember -SID 'S-1-5-32-545' -Member $user }
    if (@(Get-LocalGroupMember -SID 'S-1-5-32-544').SID.Value -contains $user.SID.Value) { throw 'Test account is an administrator' }
    & icacls.exe $fixture /grant "*$($user.SID.Value):(OI)(CI)M"
    if ($LASTEXITCODE -ne 0) { throw 'Test account cannot access proof files' }
    $credential = [pscredential]::new("$env:COMPUTERNAME\$userName", $password)
    $powershell = "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
    $process = Start-Process $powershell -Credential $credential -LoadUserProfile -WorkingDirectory $fixture -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$fixture\verify-msi-user.ps1`" -Fixture `"$fixture`"" -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Standard-user verification failed: $($process.ExitCode)" }
    $proof = Get-Content (Join-Path $fixture 'user-proof.log') -Raw
    if (($proof | Select-String 'STANDARD_USER_UNICODE_INSTALL_REMOVE_OK' -AllMatches).Matches.Count -ne 3) { throw 'Expected three standard-user Unicode cases' }
    Write-Host $proof
} finally {
    if (Test-Path $fixture) { Copy-Item "$fixture\*.log" './dist/' -ErrorAction Continue }
    if ($createdUser) { Remove-LocalUser -Name $userName }
    if ($policyChanged) {
        if ($hadDisableMSI) {
            New-ItemProperty $policyPath -Name DisableMSI -PropertyType $oldDisableMSIKind -Value $oldDisableMSI -Force | Out-Null
        } else {
            Remove-ItemProperty $policyPath -Name DisableMSI
        }
        if ($policyKeyCreated) {
            $restored = Get-Item $policyPath
            if ($restored.GetValueNames().Count -eq 0 -and $restored.GetSubKeyNames().Count -eq 0) { Remove-Item $policyPath }
        }
        Write-Host 'Original DisableMSI policy restored'
    }
}

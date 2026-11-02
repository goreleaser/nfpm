$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$profile = (Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\$($identity.User.Value)").ProfileImagePath
$local = Join-Path ([Environment]::ExpandEnvironmentVariables($profile)) 'AppData\Local'
Set-Content -Path (Join-Path $local 'nfpm-msi-user-action.txt') -Value $identity.User.Value

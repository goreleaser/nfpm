param(
    [Parameter(Mandatory)][string]$Cert,
    [Parameter(Mandatory)][string]$Signed,
    [string]$Unsigned,
    [string]$ExpectedSubject = 'CN=TestCompany, O=TestCompany, C=US'
)
$ErrorActionPreference = 'Stop'

# Validate a package signed by nfpm's built-in (pure-Go) signer with the real
# Windows trust stack. Works for both MSI and MSIX: Get-AuthenticodeSignature
# and signtool dispatch to the matching SIP, which recomputes the package
# digests (the CFB imprint for MSI, the APPX digest table for MSIX) before
# checking the certificate chain.

# Trust the throwaway cert as both a root and a publisher, matching what
# create-test-cert.ps1 does for the signtool-signed control job.
Import-Certificate -FilePath $Cert -CertStoreLocation 'Cert:\LocalMachine\Root' | Out-Null
Import-Certificate -FilePath $Cert -CertStoreLocation 'Cert:\LocalMachine\TrustedPeople' | Out-Null
Write-Host "Trusted test certificate from $Cert"

if ($Unsigned) {
    $u = Get-AuthenticodeSignature $Unsigned
    Write-Host "Get-AuthenticodeSignature($Unsigned): $($u.Status)"
    if ($u.Status -ne 'NotSigned') {
        Write-Error "$Unsigned status $($u.Status), expected NotSigned"
        exit 1
    }
}

$sig = Get-AuthenticodeSignature $Signed
Write-Host "Get-AuthenticodeSignature($Signed): $($sig.Status) - $($sig.StatusMessage)"
if ($sig.Status -ne 'Valid') {
    Write-Error "$Signed status $($sig.Status), expected Valid: $($sig.StatusMessage)"
    exit 1
}

# Prove the Valid verdict came from nfpm's signer using the test PFX rather
# than some other trusted signer.
$subject = $sig.SignerCertificate.Subject
Write-Host "Signer subject: $subject"
if ($subject -ne $ExpectedSubject) {
    Write-Error "signer subject '$subject' does not match expected '$ExpectedSubject'"
    exit 1
}

# Find signtool.exe from Windows SDK (same lookup as sign-msix.ps1).
$signtool = Get-ChildItem -Path "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Recurse -Filter signtool.exe |
    Where-Object { $_.FullName -match '\\x64\\' } |
    Sort-Object FullName -Descending |
    Select-Object -First 1

if (-not $signtool) {
    Write-Error "signtool.exe not found"
    exit 1
}

Write-Host "Using signtool: $($signtool.FullName)"
& $signtool.FullName verify /pa /v $Signed
if ($LASTEXITCODE -ne 0) {
    Write-Error "signtool verify /pa failed with exit code $LASTEXITCODE"
    exit 1
}

Write-Host "Signature validation passed for $Signed"

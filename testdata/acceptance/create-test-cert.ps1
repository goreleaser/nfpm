$ErrorActionPreference = 'Stop'

$cert = New-SelfSignedCertificate -Type Custom `
    -Subject 'CN=TestCompany, O=TestCompany, C=US' `
    -KeyUsage DigitalSignature `
    -FriendlyName 'nfpm-test' `
    -CertStoreLocation 'Cert:\CurrentUser\My' `
    -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3', '2.5.29.19={text}')

Export-PfxCertificate -Cert $cert `
    -FilePath ./dist/test.pfx `
    -Password (ConvertTo-SecureString -String 'test123' -Force -AsPlainText)

Export-Certificate -Cert $cert -FilePath ./dist/test.cer

# Self-signed cert must be trusted as both a root CA and a publisher
Import-Certificate -FilePath ./dist/test.cer `
    -CertStoreLocation 'Cert:\LocalMachine\Root'
Import-Certificate -FilePath ./dist/test.cer `
    -CertStoreLocation 'Cert:\LocalMachine\TrustedPeople'

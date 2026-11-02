Set-Content -LiteralPath (Join-Path $env:SystemRoot 'Temp\nfpm-acc-postinstall-fail.txt') -Value 'postinstall-fail-ran' -Encoding UTF8
Write-Host "nfpm postinstall: failing on purpose"
exit 1

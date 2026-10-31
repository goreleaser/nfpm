$ErrorActionPreference = 'Stop'

function Invoke-Installer([string]$name, [string]$operation, [string]$log) {
    $package = (Resolve-Path "./dist/$name.msi").Path
    $proc = Start-Process msiexec.exe -ArgumentList @($operation, "`"$package`"", '/qn', '/norestart', '/l*v', "`"$log`"") -Wait -PassThru
    return $proc.ExitCode
}

function Get-Products([string]$name) {
    return @(Get-ItemProperty @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
    ) -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -eq $name })
}

function Assert-Payload([string]$product, [string]$directory, [string]$message) {
    $products = @(Get-Products $product)
    if ($products.Count -ne 1) { throw "$product has $($products.Count) registered products" }
    $output = & (Join-Path $directory 'app.exe')
    if ($LASTEXITCODE -ne 0 -or $output -ne $message) { throw "$product has the wrong payload: $output" }
    return $products[0].PSChildName
}

foreach ($case in @(
    @{ Product = 'NfpmMsiEqual'; Old = 'equal-rc'; New = 'equal-final-allowed'; Blocked = 'equal-final-blocked'; Root = $env:ProgramFiles },
    @{ Product = 'NfpmMsiOverride'; Old = 'override-rc'; New = 'override-final'; Blocked = $null; Root = $env:ProgramFiles },
    @{ Product = 'NfpmMsiArch'; Old = 'arch-intel'; New = 'arch-x86'; Blocked = $null; Root = ${env:ProgramFiles(x86)} }
)) {
    $directory = Join-Path $case.Root $case.Product
    if (@(Get-Products $case.Product).Count -ne 0) { throw "$($case.Product) was already installed" }
    $cleanup = $case.Old
    try {
        $code = Invoke-Installer $case.Old '/i' "./dist/$($case.Old)-install.log"
        if ($code -ne 0) { throw "Installing $($case.Old) returned $code" }
        $oldCode = Assert-Payload $case.Product $directory 'nfpm-msix-test-ok'
        if ($case.Blocked) {
            $before = (Get-FileHash (Join-Path $directory 'app.exe')).Hash
            $code = Invoke-Installer $case.Blocked '/i' './dist/equal-blocked.log'
            if ($code -ne 1603) { throw "Expected equal-version rejection 1603, got $code" }
            $stillInstalled = Assert-Payload $case.Product $directory 'nfpm-msix-test-ok'
            if ($stillInstalled -ne $oldCode -or (Get-FileHash (Join-Path $directory 'app.exe')).Hash -ne $before) {
                throw 'Rejected equal-version installation changed the existing product'
            }
            Write-Host 'EQUAL_VERSION_REJECTED_ONE_PRODUCT_UNCHANGED_PAYLOAD_OK'
        }
        $code = Invoke-Installer $case.New '/i' "./dist/$($case.New)-install.log"
        if ($code -ne 0) { throw "Installing $($case.New) returned $code" }
        $cleanup = $case.New
        $newCode = Assert-Payload $case.Product $directory 'nfpm-msi-v2-test-ok'
        if ($newCode -eq $oldCode) { throw 'Major upgrade retained the old ProductCode' }
        Write-Host "$($case.Product)_ONE_PRODUCT_REPLACED_PAYLOAD_OK"
    } finally {
        $code = Invoke-Installer $cleanup '/x' "./dist/$cleanup-remove.log"
        if ($code -ne 0) { throw "Removing $cleanup returned $code" }
    }
    if ((Test-Path $directory) -or @(Get-Products $case.Product).Count -ne 0) {
        throw "$($case.Product) was not completely removed"
    }
}

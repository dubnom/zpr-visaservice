param(
    [Parameter(Mandatory = $true)][string]$Installer
)
$ErrorActionPreference = 'Stop'
$install = Join-Path $env:LOCALAPPDATA 'ZPR\EnrollmentSetup'
$state = Join-Path $env:LOCALAPPDATA 'ZPR\EnrollmentDevelopment'
$registry = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup'
if ((Test-Path $install) -or (Test-Path $state) -or (Test-Path $registry)) {
    throw 'Use a clean non-elevated Windows 11 x64 test account; existing installation/state must not be touched.'
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run without elevation.'
}
$process = Start-Process -FilePath (Resolve-Path $Installer) -ArgumentList '/S' -Wait -PassThru
if ($process.ExitCode -ne 0) { throw "Installer exit code $($process.ExitCode)" }
foreach ($name in @('zpr-enrollment-setup.exe', 'launch-setup.cmd', 'setup.example.json', 'Uninstall.exe', 'README.txt')) {
    if (!(Test-Path (Join-Path $install $name))) { throw "Missing installed file: $name" }
}
if (!(Test-Path $registry)) { throw 'Missing per-user uninstall registration.' }
$config = Join-Path $install 'setup.json'
Copy-Item (Join-Path $install 'setup.example.json') $config
$stdout = Join-Path $install 'test-stdout.txt'
$stderr = Join-Path $install 'test-stderr.txt'
$wizard = $null
try {
    $wizard = Start-Process -FilePath (Join-Path $install 'zpr-enrollment-setup.exe') `
        -ArgumentList @('-config', "`"$config`"", '-user-state') `
        -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
    $url = $null
    for ($i = 0; $i -lt 50; $i++) {
        Start-Sleep -Milliseconds 100
        if ($wizard.HasExited) { throw "Wizard exited: $(Get-Content $stderr -Raw)" }
        $text = Get-Content $stdout -Raw
        if ($text -match '(http://127\.0\.0\.1:\d+)/#session=([A-Za-z0-9_-]+)') {
            $url = $Matches[1]
            $token = $Matches[2]
            break
        }
    }
    if (!$url) { throw 'Wizard did not publish a loopback session.' }
    $headers = @{ 'X-ZPR-Setup-Session' = $token; 'Origin' = $url }
    $session = Invoke-RestMethod "$url/api/session" -Headers $headers
    if ($session.key_protection -notmatch 'Windows user-bound DPAPI') { throw 'Wrong key protection label.' }
    $body = '{"organization":"windows-test","invitation_id":"test-only-no-service-claim"}'
    Invoke-RestMethod "$url/api/prepare" -Method Post -Headers $headers -ContentType 'application/json' -Body $body | Out-Null
    $identity = Join-Path $state 'identity.dpapi'
    if (!(Test-Path $identity)) { throw 'DPAPI identity was not created.' }
    $before = (Get-FileHash $identity -Algorithm SHA256).Hash
}
finally {
    if ($wizard -and !$wizard.HasExited) {
        Stop-Process -Id $wizard.Id
        $wizard.WaitForExit()
    }
}
# Reinstall is an upgrade and must preserve operator configuration and identity.
$process = Start-Process -FilePath (Resolve-Path $Installer) -ArgumentList '/S' -Wait -PassThru
if ($process.ExitCode -ne 0 -or !(Test-Path $config) -or (Get-FileHash $identity).Hash -ne $before) {
    throw 'Upgrade lost operator configuration or identity.'
}
$process = Start-Process -FilePath (Join-Path $install 'Uninstall.exe') -ArgumentList '/S' -Wait -PassThru
for ($i = 0; $i -lt 50 -and ((Test-Path $registry) -or
    (Test-Path (Join-Path $install 'zpr-enrollment-setup.exe')) -or
    (Test-Path (Join-Path $install 'Uninstall.exe'))); $i++) {
    Start-Sleep -Milliseconds 100
}
if ((Test-Path $registry) -or (Test-Path (Join-Path $install 'zpr-enrollment-setup.exe'))) { throw 'Uninstall incomplete.' }
if (!(Test-Path $config) -or (Get-FileHash $identity).Hash -ne $before) { throw 'Uninstall lost protected state/configuration.' }
# Only files created by this test, in the initially absent installation/state.
Remove-Item $identity, $config, $stdout, $stderr
Remove-Item $state
Remove-Item $install
Write-Output 'Windows per-user install, setup, upgrade and preserved-state uninstall passed. No service claim or adapter connection was attempted.'

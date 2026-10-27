$ErrorActionPreference = 'Stop'
try {
    $OutputEncoding = [Console]::InputEncoding = [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
    $payload = [Console]::In.ReadToEnd()
    $guardProgram = Join-Path $PSScriptRoot '../bin/windows-amd64/issue-pr-guard.exe'
    if (-not (Test-Path -LiteralPath $guardProgram)) { $guardProgram = Join-Path $PSScriptRoot '../bin/issue-pr-guard.exe' }
    $payload | & $guardProgram
    if ($LASTEXITCODE -ne 0) { throw 'Guard failed.' }
} catch {
    [Console]::Error.WriteLine('Issue / PR guard: check failed; tool call denied. ' + $_.Exception.Message)
    exit 2
}
exit 0

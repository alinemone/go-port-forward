# Install pf (go-port-forward) on Windows. In PowerShell:
#
#   irm https://raw.githubusercontent.com/alinemone/go-port-forward/main/install.ps1 | iex
#
# Options (environment variables):
#   PF_VERSION      release tag to install, e.g. v3.2.1 (default: latest)
#   PF_INSTALL_DIR  where to put pf.exe (default: ~\.pf)
#   PF_NO_PATH=1    don't add the directory to your user PATH
#
# The binary is checked against the release's SHA256SUMS.txt before it is
# installed. No administrator rights are needed.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is far faster without the progress bar

    $repo = 'alinemone/go-port-forward'
    $version = if ($env:PF_VERSION) { $env:PF_VERSION } else { 'latest' }
    $dir = if ($env:PF_INSTALL_DIR) { $env:PF_INSTALL_DIR } else { Join-Path $HOME '.pf' }

    # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub refuses.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    # pf is built for amd64; Windows on ARM runs it through x64 emulation.
    $asset = 'pf-windows-amd64.exe'
    $base = if ($version -eq 'latest') {
        "https://github.com/$repo/releases/latest/download"
    } else {
        "https://github.com/$repo/releases/download/$version"
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("pf-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    try {
        Write-Host "-> downloading $asset ($version)"
        $exe = Join-Path $tmp 'pf.exe'
        $sums = Join-Path $tmp 'SHA256SUMS.txt'
        foreach ($d in @(@("$base/$asset", $exe), @("$base/SHA256SUMS.txt", $sums))) {
            try { Invoke-WebRequest -UseBasicParsing -Uri $d[0] -OutFile $d[1] }
            catch { throw "download failed: $($d[0]) ($($_.Exception.Message))" }
        }

        $line = Get-Content $sums | Where-Object { ($_ -split '\s+')[1] -eq $asset } | Select-Object -First 1
        if (-not $line) { throw "$asset is not listed in SHA256SUMS.txt" }
        $expected = ($line -split '\s+')[0].ToLower()
        # .NET directly: Get-FileHash isn't available on every Windows setup.
        $stream = [IO.File]::OpenRead($exe)
        try { $hash = [Security.Cryptography.SHA256]::Create().ComputeHash($stream) } finally { $stream.Dispose() }
        $actual = -join ($hash | ForEach-Object { $_.ToString('x2') })
        if ($expected -ne $actual) {
            throw "checksum mismatch for $asset - not installing (expected $expected, got $actual)"
        }
        Write-Host '[ok] checksum verified'

        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $target = Join-Path $dir 'pf.exe'
        if (Test-Path $target) {
            # A running pf.exe can't be overwritten, but it can be renamed:
            # move it aside and the new one takes its place.
            $old = "$target.old"
            Remove-Item -Force -ErrorAction SilentlyContinue $old
            Move-Item -Force $target $old
        }
        Move-Item -Force $exe $target
        Write-Host "[ok] installed $target"
        & $target version

        if ($env:PF_NO_PATH -ne '1') {
            $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
            $entries = @($userPath -split ';' | Where-Object { $_ })
            if ($entries -notcontains $dir) {
                [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
                Write-Host "[ok] added $dir to your user PATH"
            }
            if (($env:Path -split ';') -notcontains $dir) {
                $env:Path = "$env:Path;$dir" # this window too
            }
            Write-Host ''
            Write-Host 'Run: pf   (other terminals that were already open need to be reopened)'
        }
    }
    catch {
        Write-Host "pf install: $_" -ForegroundColor Red
    }
    finally {
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
    }
}

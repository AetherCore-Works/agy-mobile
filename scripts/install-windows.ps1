param(
    [switch]$SkipBuild,
    [switch]$Start,
    [switch]$AutoStart,
    [int]$Port = 8088,
    [string]$WorkspaceDir = ""
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
if (-not $WorkspaceDir) {
    $WorkspaceDir = $RepoRoot
}

$InstallBinDir = Join-Path $env:LOCALAPPDATA "agy\bin"
$DataDir = Join-Path $env:LOCALAPPDATA "agy-gateway"
$LogDir = Join-Path $DataDir "logs"
$EnvFile = Join-Path $DataDir ".env"

New-Item -ItemType Directory -Force -Path $InstallBinDir, $DataDir, $LogDir | Out-Null

Write-Host "==> [1/4] Parando instancias antigas do AGY Gateway / Cockpit..." -ForegroundColor Cyan
Get-Process -Name "agy-gateway", "agy-cockpit" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
$ttydConn = Get-NetTCPConnection -LocalPort 7681 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if ($ttydConn) {
    Stop-Process -Id $ttydConn.OwningProcess -Force -ErrorAction SilentlyContinue
}
Start-Sleep -Milliseconds 400

if (-not $SkipBuild) {
    Write-Host "==> [2/4] Compilando frontend Svelte 5 e binarios Go..." -ForegroundColor Cyan
    Push-Location (Join-Path $RepoRoot "frontend")
    try {
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "npm run build falhou ($LASTEXITCODE)" }
    } finally {
        Pop-Location
    }

    $env:CGO_ENABLED = "0"
    Push-Location (Join-Path $RepoRoot "backend-go")
    try {
        go build -o agy-gateway.exe ./cmd/agy-gateway
        if ($LASTEXITCODE -ne 0) { throw "go build agy-gateway.exe falhou ($LASTEXITCODE)" }
    } finally {
        Pop-Location
    }

    Push-Location (Join-Path $RepoRoot "packages\cockpit")
    try {
        go build -o agy-cockpit.exe ./cmd/agy-cockpit
        if ($LASTEXITCODE -ne 0) { throw "go build agy-cockpit.exe falhou ($LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
}

Write-Host "==> [3/4] Instalando binarios, .env e atalhos silenciosos em $InstallBinDir..." -ForegroundColor Cyan
$GatewaySrc = Join-Path $RepoRoot "backend-go\agy-gateway.exe"
$CockpitSrc = Join-Path $RepoRoot "packages\cockpit\agy-cockpit.exe"
$IconSrc = Join-Path $RepoRoot "backend-go\cmd\agy-gateway\app.ico"

Copy-Item -Path $GatewaySrc -Destination (Join-Path $InstallBinDir "agy-gateway.exe") -Force
if (Test-Path $CockpitSrc) {
    Copy-Item -Path $CockpitSrc -Destination (Join-Path $InstallBinDir "agy-cockpit.exe") -Force
}
if (Test-Path $IconSrc) {
    Copy-Item -Path $IconSrc -Destination (Join-Path $InstallBinDir "agy-gateway.ico") -Force
}

$ResolvedAgyBin = Join-Path $env:USERPROFILE ".gemini\bin\agy.exe"
if (-not (Test-Path $ResolvedAgyBin)) {
    $cmdAgy = Get-Command agy -ErrorAction SilentlyContinue
    if ($cmdAgy) { $ResolvedAgyBin = $cmdAgy.Source } else { $ResolvedAgyBin = "agy" }
}

if (-not (Test-Path $EnvFile)) {
    $defaultEnv = @"
GATEWAY_MODE=standalone
PORT=$Port
ENABLE_TERMINAL=true
ENABLE_TELEMETRY=true
ENABLE_COCKPIT=true
ENABLE_OLLAMA=true
COCKPIT_ADDR=127.0.0.1:8099
COCKPIT_URL=http://127.0.0.1:8099
COCKPIT_ALLOW_POWER=true
TTYD_WS_URL=ws://127.0.0.1:7681/terminal/ws
AGY_BIN=$ResolvedAgyBin
AGY_WORKSPACE=$WorkspaceDir
"@
    Set-Content -Path $EnvFile -Value $defaultEnv -Encoding UTF8
} else {
    $curEnv = Get-Content $EnvFile -Raw -ErrorAction SilentlyContinue
    if ($curEnv -and -not ($curEnv -match "(?m)^AGY_WORKSPACE=")) {
        Add-Content -Path $EnvFile -Value "`nAGY_WORKSPACE=$WorkspaceDir" -Encoding UTF8
    }
}

$DaemonScriptPath = Join-Path $InstallBinDir "start-agy-daemons.ps1"
$DaemonScriptBody = @'
param(
    [switch]$OpenApp,
    [int]$Port = __PORT__,
    [string]$WorkspaceDir = "__WORKSPACE__"
)

$ErrorActionPreference = "SilentlyContinue"
$BinDir = $PSScriptRoot
$DataDir = Join-Path $env:LOCALAPPDATA "agy-gateway"
$LogDir = Join-Path $DataDir "logs"
$EnvFile = Join-Path $DataDir ".env"
New-Item -ItemType Directory -Force -Path $DataDir, $LogDir | Out-Null

if (Test-Path $EnvFile) {
    Get-Content $EnvFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $idx = $line.IndexOf("=")
            $k = $line.Substring(0, $idx).Trim()
            $v = $line.Substring($idx + 1).Trim().Trim('"').Trim("'")
            if ($k) { [Environment]::SetEnvironmentVariable($k, $v, "Process") }
        }
    }
}

if ($env:PORT) { $Port = [int]$env:PORT }

function Test-LocalPort([int]$p) {
    try {
        $c = New-Object System.Net.Sockets.TcpClient
        $ar = $c.BeginConnect("127.0.0.1", $p, $null, $null)
        $ok = $ar.AsyncWaitHandle.WaitOne(250, $false)
        if ($ok) { $c.EndConnect($ar) }
        $c.Close()
        return $ok
    } catch {
        return $false
    }
}

$workDir = if (Test-Path $WorkspaceDir) { $WorkspaceDir } else { $DataDir }

if (-not (Test-LocalPort 7681)) {
    $ttydCmd = Get-Command ttyd -ErrorAction SilentlyContinue
    if ($ttydCmd) {
        $shellExe = (Get-Command pwsh -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source -First 1)
        if (-not $shellExe) { $shellExe = "powershell.exe" }
        Start-Process -FilePath $ttydCmd.Source -ArgumentList "-p", "7681", "-b", "/terminal", "-W", "-t", "fontSize=14", "-w", $workDir, $shellExe, "-NoLogo" -WorkingDirectory $workDir -WindowStyle Hidden
    }
}

$cockpitExe = Join-Path $BinDir "agy-cockpit.exe"
if ((Test-Path $cockpitExe) -and -not (Test-LocalPort 8099)) {
    Start-Process -FilePath $cockpitExe -WorkingDirectory $BinDir -WindowStyle Hidden
}

$gatewayExe = Join-Path $BinDir "agy-gateway.exe"
if ((Test-Path $gatewayExe) -and -not (Test-LocalPort $Port)) {
    $env:NO_SINGLE_INSTANCE_OPEN = "1"
    $outLog = Join-Path $LogDir "gateway.log"
    $errLog = Join-Path $LogDir "gateway.err.log"
    Start-Process -FilePath $gatewayExe -WorkingDirectory $workDir -WindowStyle Hidden -RedirectStandardOutput $outLog -RedirectStandardError $errLog
}

if ($OpenApp) {
    $url = "http://127.0.0.1:$Port"
    for ($i = 0; $i -lt 25; $i++) {
        if (Test-LocalPort $Port) { break }
        Start-Sleep -Milliseconds 200
    }
    $progId = (Get-ItemProperty "HKCU:\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice" -ErrorAction SilentlyContinue).ProgId
    $edgePaths = @(
        (Join-Path ${env:ProgramFiles(x86)} "Microsoft\Edge\Application\msedge.exe"),
        (Join-Path $env:ProgramFiles "Microsoft\Edge\Application\msedge.exe")
    )
    $chromePaths = @(
        (Join-Path $env:ProgramFiles "Google\Chrome\Application\chrome.exe"),
        (Join-Path $env:LOCALAPPDATA "Google\Chrome\Application\chrome.exe")
    )
    $preferred = if ($progId -match "MSEdge") { $edgePaths + $chromePaths } elseif ($progId -match "Chrome") { $chromePaths + $edgePaths } else { @() }
    $opened = $false
    foreach ($b in $preferred) {
        if ($b -and (Test-Path $b)) {
            Start-Process -FilePath $b -ArgumentList "--app=$url"
            $opened = $true
            break
        }
    }
    if (-not $opened) {
        Start-Process $url
    }
}
'@
$DaemonScriptBody = $DaemonScriptBody.Replace("__PORT__", [string]$Port).Replace("__WORKSPACE__", $WorkspaceDir)
Set-Content -Path $DaemonScriptPath -Value $DaemonScriptBody -Encoding UTF8

$PsRunner = (Get-Command pwsh -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source -First 1)
if (-not $PsRunner) { $PsRunner = "powershell.exe" }

$StartupVbs = Join-Path $InstallBinDir "agy-startup.vbs"
$OpenVbs = Join-Path $InstallBinDir "agy-open.vbs"
if (Test-Path $OpenVbs) {
    Remove-Item $OpenVbs -Force -ErrorAction SilentlyContinue
}

$Wsh = New-Object -ComObject WScript.Shell
$StartupShortcutLegacy = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup\AGY Mobile Gateway.lnk"
$TaskName = "AGY-Mobile-Gateway"
$TaskCreated = $false

if ($AutoStart) {
    Set-Content -Path $StartupVbs -Value ("CreateObject(""WScript.Shell"").Run """"""$PsRunner"""" -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File """"$DaemonScriptPath"""""", 0, False") -Encoding ASCII
    if (Test-Path $StartupShortcutLegacy) {
        Remove-Item $StartupShortcutLegacy -Force -ErrorAction SilentlyContinue
    }
    try {
        $action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\wscript.exe" -Argument "//B //NoLogo `"$StartupVbs`"" -WorkingDirectory $InstallBinDir
        $trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
        $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
        $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Seconds 0) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -StartWhenAvailable
        Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description "AGY Mobile Gateway, Cockpit e Terminal Web em segundo plano" -Force | Out-Null
        $TaskCreated = $true
        Write-Host "    Tarefa agendada '$TaskName' registrada para iniciar no logon do Windows." -ForegroundColor Green
    } catch {
        $scStartup = $Wsh.CreateShortcut($StartupShortcutLegacy)
        $scStartup.TargetPath = "$env:SystemRoot\System32\wscript.exe"
        $scStartup.Arguments = "//B //NoLogo `"$StartupVbs`""
        $scStartup.WorkingDirectory = $InstallBinDir
        if (Test-Path (Join-Path $InstallBinDir "agy-gateway.ico")) {
            $scStartup.IconLocation = (Join-Path $InstallBinDir "agy-gateway.ico")
        }
        $scStartup.Save()
        Write-Host "    Atalho de Startup criado em $StartupShortcutLegacy" -ForegroundColor Yellow
    }
} else {
    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
    }
    if (Test-Path $StartupShortcutLegacy) {
        Remove-Item $StartupShortcutLegacy -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $StartupVbs) {
        Remove-Item $StartupVbs -Force -ErrorAction SilentlyContinue
    }
    Write-Host "    Auto-start ao ligar/logar removido (o AGY so inicia quando voce clicar)." -ForegroundColor Green
}

$UserAppShortcut = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\AGY.lnk"
$scUserApp = $Wsh.CreateShortcut($UserAppShortcut)
$scUserApp.TargetPath = (Join-Path $InstallBinDir "agy-gateway.exe")
$scUserApp.Arguments = "--console"
$scUserApp.WorkingDirectory = $WorkspaceDir
if (Test-Path (Join-Path $InstallBinDir "agy-gateway.ico")) {
    $scUserApp.IconLocation = (Join-Path $InstallBinDir "agy-gateway.ico")
}
$scUserApp.Save()

$ProgramDataShortcut = "C:\ProgramData\Microsoft\Windows\Start Menu\AGY.lnk"
if (Test-Path $ProgramDataShortcut) {
    try {
        $scPub = $Wsh.CreateShortcut($ProgramDataShortcut)
        $scPub.TargetPath = (Join-Path $InstallBinDir "agy-gateway.exe")
        $scPub.Arguments = "--console"
        $scPub.WorkingDirectory = $WorkspaceDir
        if (Test-Path (Join-Path $InstallBinDir "agy-gateway.ico")) {
            $scPub.IconLocation = (Join-Path $InstallBinDir "agy-gateway.ico")
        }
        $scPub.Save()
        Write-Host "    Atalho $ProgramDataShortcut atualizado para abrir o painel interativo." -ForegroundColor Green
    } catch {
        Write-Host "    Aviso: sem permissao de Admin para editar $ProgramDataShortcut (o .exe alvo ja abre o painel interativo diretamente)." -ForegroundColor Yellow
    }
}

if ($Start) {
    Write-Host "==> [4/4] Iniciando servicos ocultos e validando APIs..." -ForegroundColor Cyan
    if ($TaskCreated) {
        Start-ScheduledTask -TaskName $TaskName
    } else {
        & $DaemonScriptPath
    }
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $health = Invoke-RestMethod "http://127.0.0.1:$Port/api/health"
            if ($health.ok) { $ready = $true; break }
        } catch {
            Start-Sleep -Milliseconds 300
        }
    }
    if (-not $ready) { throw "Gateway nao respondeu em http://127.0.0.1:$Port/api/health" }
    $feats = Invoke-RestMethod "http://127.0.0.1:$Port/api/features"
    $slashes = Invoke-RestMethod "http://127.0.0.1:$Port/api/slash-commands"
    Write-Host ("    Health: " + ($health | ConvertTo-Json -Compress)) -ForegroundColor Green
    Write-Host ("    Terminal=$($feats.features.terminal) Telemetry=$($feats.features.telemetry) Cockpit=$($feats.features.cockpit) SlashCommands=$($slashes.commands.Count)") -ForegroundColor Green
}

Write-Host "==> Instalacao Windows concluida! Clique no atalho AGY para abrir o painel no terminal." -ForegroundColor Green

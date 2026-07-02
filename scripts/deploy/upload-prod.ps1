[CmdletBinding()]
param(
    [string]$ConfigPath = "",

    [string]$Rollout = "",

    [string]$LocalRolloutDir = "",

    [ValidateSet("api-web", "migration")]
    [string]$DeployMode = "api-web"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..\..")

function Get-JsonValue {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Object,

        [Parameter(Mandatory = $true)]
        [string]$Name,

        [object]$DefaultValue = $null
    )

    $property = $Object.PSObject.Properties[$Name]
    if ($property) {
        return $property.Value
    }

    return $DefaultValue
}

function Assert-CommandExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$CommandName
    )

    if (-not (Get-Command $CommandName -ErrorAction SilentlyContinue)) {
        throw "Required command is not available on PATH: $CommandName"
    }
}

function Assert-RolloutName {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Value
    )

    if ($Value -notmatch "^[A-Za-z0-9._-]+$") {
        throw "Rollout may contain only letters, numbers, dot, underscore, and hyphen: $Value"
    }
}

function Quote-Bash {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Value
    )

    return "'" + $Value.Replace("'", "'""'""'") + "'"
}

function Get-ArtifactHash {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Invoke-SshCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RemoteCommand
    )

    $args = @()
    if ($script:SshPort) {
        $args += @("-p", [string]$script:SshPort)
    }

    if ($script:SshKeyPath) {
        $args += @("-i", $script:SshKeyPath)
    }

    $args += @($script:SshTarget, $RemoteCommand)

    Write-Host ("+ ssh {0}" -f $RemoteCommand)
    & ssh @args
    if ($LASTEXITCODE -ne 0) {
        throw "ssh command failed with exit code $LASTEXITCODE."
    }
}

function Invoke-ScpUpload {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$LocalPaths,

        [Parameter(Mandatory = $true)]
        [string]$RemoteDirectory
    )

    $args = @()
    if ($script:SshPort) {
        $args += @("-P", [string]$script:SshPort)
    }

    if ($script:SshKeyPath) {
        $args += @("-i", $script:SshKeyPath)
    }

    $args += $LocalPaths
    $args += "${script:SshTarget}:$RemoteDirectory/"

    Write-Host ("+ scp {0} {1}:..." -f ($LocalPaths -join " "), $script:SshTarget)
    & scp @args
    if ($LASTEXITCODE -ne 0) {
        throw "scp failed with exit code $LASTEXITCODE."
    }
}

function Get-DefaultLocalRolloutDir {
    $root = Join-Path $RepoRoot "tmp\deploy"
    if (-not (Test-Path -LiteralPath $root)) {
        throw "No local deploy output found. Run scripts\deploy\build-prod.ps1 first."
    }

    $latest = Get-ChildItem -LiteralPath $root -Directory |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1

    if (-not $latest) {
        throw "No rollout directories found under $root."
    }

    return $latest.FullName
}

Assert-CommandExists -CommandName "ssh"
Assert-CommandExists -CommandName "scp"

if ([string]::IsNullOrWhiteSpace($ConfigPath)) {
    $ConfigPath = Join-Path $ScriptDir "deploy-config.local.json"
}

if (-not (Test-Path -LiteralPath $ConfigPath)) {
    throw "Deployment config not found: $ConfigPath. Copy deploy-config.sample.json to deploy-config.local.json and fill in non-secret VPS values."
}

$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json

$sshHost = [string](Get-JsonValue -Object $config -Name "sshHost")
$sshUser = [string](Get-JsonValue -Object $config -Name "sshUser")
$script:SshPort = Get-JsonValue -Object $config -Name "sshPort" -DefaultValue 22
$script:SshKeyPath = [string](Get-JsonValue -Object $config -Name "sshKeyPath" -DefaultValue "")
$remoteBackupRoot = [string](Get-JsonValue -Object $config -Name "remoteBackupRoot" -DefaultValue "/opt/meetingroombooker/backups")

if ([string]::IsNullOrWhiteSpace($sshHost)) {
    throw "deploy config is missing sshHost."
}

if ([string]::IsNullOrWhiteSpace($sshUser)) {
    throw "deploy config is missing sshUser."
}

if ($script:SshKeyPath) {
    $script:SshKeyPath = (Resolve-Path $script:SshKeyPath).Path
}

$script:SshTarget = "$sshUser@$sshHost"

if ([string]::IsNullOrWhiteSpace($LocalRolloutDir)) {
    if ([string]::IsNullOrWhiteSpace($Rollout)) {
        $LocalRolloutDir = Get-DefaultLocalRolloutDir
    }
    else {
        Assert-RolloutName -Value $Rollout
        $LocalRolloutDir = Join-Path $RepoRoot "tmp\deploy\$Rollout"
    }
}

$LocalRolloutDir = (Resolve-Path $LocalRolloutDir).Path
$ArtifactsDir = Join-Path $LocalRolloutDir "artifacts"
$ManifestPath = Join-Path $ArtifactsDir "release-manifest.json"

if (-not (Test-Path -LiteralPath $ManifestPath)) {
    throw "release-manifest.json not found: $ManifestPath"
}

$manifest = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
$manifestRollout = [string](Get-JsonValue -Object $manifest -Name "rollout")
$manifestDeployMode = [string](Get-JsonValue -Object $manifest -Name "deployMode")

if ([string]::IsNullOrWhiteSpace($Rollout)) {
    $Rollout = $manifestRollout
}

Assert-RolloutName -Value $Rollout

if ($manifestRollout -and $manifestRollout -ne $Rollout) {
    throw "Requested rollout '$Rollout' does not match manifest rollout '$manifestRollout'."
}

if ($manifestDeployMode -and $manifestDeployMode -ne $DeployMode) {
    throw "Requested deploy mode '$DeployMode' does not match manifest deploy mode '$manifestDeployMode'."
}

$requiredArtifactNames = @("api.zip", "web.zip")
if ($DeployMode -eq "migration") {
    $requiredArtifactNames += "efbundle-linux-x64"
}

$requiredArtifactNames += "release-manifest.json"

$artifactPaths = @()
foreach ($name in $requiredArtifactNames) {
    $path = Join-Path $ArtifactsDir $name
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Required artifact is missing: $path"
    }

    $artifactPaths += $path
}

$scriptPaths = @(
    (Join-Path $ScriptDir "remote-preflight.sh"),
    (Join-Path $ScriptDir "deploy-prod.sh"),
    (Join-Path $ScriptDir "rollback-prod.sh")
)

foreach ($path in $scriptPaths) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Required remote script is missing: $path"
    }
}

$remoteRolloutDir = "$remoteBackupRoot/$Rollout"
$remoteArtifactsDir = "$remoteRolloutDir/artifacts"
$remoteScriptsDir = "$remoteRolloutDir/scripts"

Write-Host ""
Write-Host "Upload target: $script:SshTarget"
Write-Host "Remote rollout: $remoteRolloutDir"
Write-Host "Deploy mode:    $DeployMode"

Invoke-SshCommand -RemoteCommand ("mkdir -p {0} {1}" -f (Quote-Bash $remoteArtifactsDir), (Quote-Bash $remoteScriptsDir))
Invoke-ScpUpload -LocalPaths $artifactPaths -RemoteDirectory $remoteArtifactsDir
Invoke-ScpUpload -LocalPaths $scriptPaths -RemoteDirectory $remoteScriptsDir
Invoke-SshCommand -RemoteCommand ("chmod 700 {0}/*.sh" -f (Quote-Bash $remoteScriptsDir))

if ($DeployMode -eq "migration") {
    Invoke-SshCommand -RemoteCommand ("chmod 700 {0}" -f (Quote-Bash "$remoteArtifactsDir/efbundle-linux-x64"))
}

$checkLines = foreach ($name in $requiredArtifactNames) {
    $path = Join-Path $ArtifactsDir $name
    "{0}  {1}" -f (Get-ArtifactHash -Path $path), $name
}

$checkPayload = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes(($checkLines -join "`n") + "`n"))
$remoteCheckCommand = "cd $(Quote-Bash $remoteArtifactsDir) && printf %s $(Quote-Bash $checkPayload) | base64 -d | sha256sum --check --strict"

Invoke-SshCommand -RemoteCommand $remoteCheckCommand

$sshOptionsForDisplay = @()
if ($script:SshPort) {
    $sshOptionsForDisplay += @("-p", [string]$script:SshPort)
}

if ($script:SshKeyPath) {
    $sshOptionsForDisplay += @("-i", "`"$script:SshKeyPath`"")
}

$sshPrefix = "ssh"
if ($sshOptionsForDisplay.Count -gt 0) {
    $sshPrefix = "$sshPrefix $($sshOptionsForDisplay -join " ")"
}

$preflightCommand = "sudo env BACKUP_DIR=$remoteRolloutDir DEPLOY_MODE=$DeployMode bash $remoteScriptsDir/remote-preflight.sh"
$deployCommand = "sudo env BACKUP_DIR=$remoteRolloutDir DEPLOY_MODE=$DeployMode bash $remoteScriptsDir/deploy-prod.sh"
$rollbackCommand = "sudo env BACKUP_DIR=$remoteRolloutDir bash $remoteScriptsDir/rollback-prod.sh"

Write-Host ""
Write-Host "Upload complete. No production service or database changes were made."
Write-Host ""
Write-Host "Next commands:"
Write-Host "  $sshPrefix $script:SshTarget `"$preflightCommand`""
Write-Host "  $sshPrefix $script:SshTarget `"$deployCommand`""
Write-Host ""
Write-Host "Rollback command, only if needed after a failed deployment:"
Write-Host "  $sshPrefix $script:SshTarget `"$rollbackCommand`""

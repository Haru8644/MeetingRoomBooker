[CmdletBinding()]
param(
    [string]$Rollout = "",

    [ValidateSet("api-web", "migration")]
    [string]$DeployMode = "api-web",

    [string]$MigrationBundlePath = "",

    [string]$Configuration = "Release",

    [switch]$AllowDirty
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$SlnPath = Join-Path $RepoRoot "MeetingRoomBooker.slnx"
$ApiProject = Join-Path $RepoRoot "MeetingRoomBooker.Api\MeetingRoomBooker.Api.csproj"
$WebProject = Join-Path $RepoRoot "MeetingRoomBooker.Web\MeetingRoomBooker.Web.csproj"

function Invoke-Step {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,

        [Parameter(Mandatory = $true)]
        [scriptblock]$Action
    )

    Write-Host ""
    Write-Host "==> $Name"
    & $Action
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,

        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    Write-Host ("+ {0} {1}" -f $FilePath, ($Arguments -join " "))
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw ("Command failed with exit code {0}: {1} {2}" -f $LASTEXITCODE, $FilePath, ($Arguments -join " "))
    }
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

function Assert-NoDatabaseFiles {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory
    )

    $databaseFiles = Get-ChildItem -LiteralPath $Directory -Recurse -File |
        Where-Object { $_.Name -match "\.db(-shm|-wal)?$" }

    if ($databaseFiles) {
        $paths = $databaseFiles | ForEach-Object { $_.FullName }
        throw "Publish output contains SQLite database files and must not be deployed:`n$($paths -join "`n")"
    }
}

function Get-ArtifactHash {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

Set-Location $RepoRoot

if ([string]::IsNullOrWhiteSpace($Rollout)) {
    $Rollout = Get-Date -Format "yyyyMMdd-HHmmss"
}

Assert-RolloutName -Value $Rollout
Assert-CommandExists -CommandName "git"
Assert-CommandExists -CommandName "dotnet"

Invoke-Step "Check git state" {
    $gitStatus = & git status --porcelain
    if ($LASTEXITCODE -ne 0) {
        throw "git status failed."
    }

    if ($gitStatus -and -not $AllowDirty) {
        Write-Host $gitStatus
        throw "Working tree is not clean. Commit/stash changes or rerun with -AllowDirty for an intentional local build."
    }

    $script:GitCommit = (& git rev-parse HEAD).Trim()
    $script:GitBranch = (& git rev-parse --abbrev-ref HEAD).Trim()
    $script:GitDirty = [bool]$gitStatus

    Write-Host "Branch: $script:GitBranch"
    Write-Host "Commit: $script:GitCommit"
    Write-Host "Dirty:  $script:GitDirty"
}

$OutputRoot = Join-Path $RepoRoot "tmp\deploy\$Rollout"
$PublishRoot = Join-Path $OutputRoot "publish"
$ArtifactsDir = Join-Path $OutputRoot "artifacts"
$ApiPublishDir = Join-Path $PublishRoot "api"
$WebPublishDir = Join-Path $PublishRoot "web"
$WebWwwrootDir = Join-Path $WebPublishDir "wwwroot"

if (Test-Path -LiteralPath $OutputRoot) {
    throw "Rollout output already exists: $OutputRoot"
}

New-Item -ItemType Directory -Path $ApiPublishDir -Force | Out-Null
New-Item -ItemType Directory -Path $WebPublishDir -Force | Out-Null
New-Item -ItemType Directory -Path $ArtifactsDir -Force | Out-Null

Invoke-Step "Restore" {
    Invoke-Checked -FilePath "dotnet" -Arguments @("restore", $SlnPath)
}

Invoke-Step "Build" {
    Invoke-Checked -FilePath "dotnet" -Arguments @("build", $SlnPath, "--configuration", $Configuration, "--no-restore")
}

Invoke-Step "Test" {
    Invoke-Checked -FilePath "dotnet" -Arguments @("test", $SlnPath, "--configuration", $Configuration, "--no-build")
}

Invoke-Step "Publish API" {
    Invoke-Checked -FilePath "dotnet" -Arguments @("publish", $ApiProject, "--configuration", $Configuration, "--output", $ApiPublishDir, "--no-restore")

    $apiDll = Join-Path $ApiPublishDir "MeetingRoomBooker.Api.dll"
    if (-not (Test-Path -LiteralPath $apiDll)) {
        throw "API publish output is missing MeetingRoomBooker.Api.dll: $apiDll"
    }

    Assert-NoDatabaseFiles -Directory $ApiPublishDir
}

Invoke-Step "Publish Web" {
    Invoke-Checked -FilePath "dotnet" -Arguments @("publish", $WebProject, "--configuration", $Configuration, "--output", $WebPublishDir, "--no-restore")

    $webIndex = Join-Path $WebWwwrootDir "index.html"
    $webFramework = Join-Path $WebWwwrootDir "_framework"

    if (-not (Test-Path -LiteralPath $webIndex)) {
        throw "Web publish output is missing wwwroot\index.html: $webIndex"
    }

    if (-not (Test-Path -LiteralPath $webFramework)) {
        throw "Web publish output is missing wwwroot\_framework: $webFramework"
    }

    Assert-NoDatabaseFiles -Directory $WebPublishDir
}

$EfBundlePath = Join-Path $ArtifactsDir "efbundle-linux-x64"

if ($DeployMode -eq "migration") {
    if (-not [string]::IsNullOrWhiteSpace($MigrationBundlePath)) {
        Invoke-Step "Use existing EF Core migration bundle" {
            if (-not (Test-Path -LiteralPath $MigrationBundlePath -PathType Leaf)) {
                throw "MigrationBundlePath must point to an existing file: $MigrationBundlePath"
            }

            $sourceBundle = (Resolve-Path -LiteralPath $MigrationBundlePath).Path
            Copy-Item -LiteralPath $sourceBundle -Destination $EfBundlePath -Force
        }
    }
    else {
        Invoke-Step "Build EF Core migration bundle" {
            try {
                Invoke-Checked -FilePath "dotnet" -Arguments @("ef", "--version")
                Invoke-Checked -FilePath "dotnet" -Arguments @(
                    "ef",
                    "migrations",
                    "bundle",
                    "--project",
                    $ApiProject,
                    "--startup-project",
                    $ApiProject,
                    "--configuration",
                    $Configuration,
                    "--runtime",
                    "linux-x64",
                    "--self-contained",
                    "--output",
                    $EfBundlePath,
                    "--force"
                )
            }
            catch {
                throw @"
Failed to create the linux-x64 EF Core migration bundle.

On Windows, this can fail because the SQLite native library e_sqlite3 cannot be loaded while creating a linux-x64 bundle. Create the bundle on Linux, WSL, or GitHub Actions, then rerun this script with -MigrationBundlePath pointing to that efbundle-linux-x64 file.

Original error:
$($_.Exception.Message)
"@
            }
        }
    }

    if (-not (Test-Path -LiteralPath $EfBundlePath -PathType Leaf)) {
        throw "EF Core migration bundle was not created: $EfBundlePath"
    }
}
else {
    Invoke-Step "Skip EF Core migration bundle" {
        Write-Host "DeployMode=api-web, so efbundle-linux-x64 is not created and will not be included in the manifest."
    }
}

$ApiZipPath = Join-Path $ArtifactsDir "api.zip"
$WebZipPath = Join-Path $ArtifactsDir "web.zip"
$ManifestPath = Join-Path $ArtifactsDir "release-manifest.json"

Invoke-Step "Create deploy archives" {
    Compress-Archive -Path (Join-Path $ApiPublishDir "*") -DestinationPath $ApiZipPath -Force
    Compress-Archive -Path (Join-Path $WebWwwrootDir "*") -DestinationPath $WebZipPath -Force

    if (-not (Test-Path -LiteralPath $ApiZipPath)) {
        throw "API archive was not created: $ApiZipPath"
    }

    if (-not (Test-Path -LiteralPath $WebZipPath)) {
        throw "Web archive was not created: $WebZipPath"
    }
}

Invoke-Step "Write release manifest" {
    $artifacts = @(
        [ordered]@{
            name = "api.zip"
            sha256 = Get-ArtifactHash -Path $ApiZipPath
            bytes = (Get-Item -LiteralPath $ApiZipPath).Length
        },
        [ordered]@{
            name = "web.zip"
            sha256 = Get-ArtifactHash -Path $WebZipPath
            bytes = (Get-Item -LiteralPath $WebZipPath).Length
        }
    )

    if ($DeployMode -eq "migration") {
        $artifacts += [ordered]@{
            name = "efbundle-linux-x64"
            sha256 = Get-ArtifactHash -Path $EfBundlePath
            bytes = (Get-Item -LiteralPath $EfBundlePath).Length
        }
    }

    $manifest = [ordered]@{
        application = "MeetingRoomBooker"
        rollout = $Rollout
        deployMode = $DeployMode
        createdAtUtc = (Get-Date).ToUniversalTime().ToString("o")
        git = [ordered]@{
            branch = $script:GitBranch
            commit = $script:GitCommit
            dirty = $script:GitDirty
        }
        build = [ordered]@{
            configuration = $Configuration
            apiProject = "MeetingRoomBooker.Api/MeetingRoomBooker.Api.csproj"
            webProject = "MeetingRoomBooker.Web/MeetingRoomBooker.Web.csproj"
            solution = "MeetingRoomBooker.slnx"
        }
        artifacts = $artifacts
        productionPaths = [ordered]@{
            apiServiceName = "meetingroombooker-api.service"
            apiPath = "/opt/meetingroombooker/api"
            dbPath = "/opt/meetingroombooker/api/app.db"
            webRoot = "/var/www/meetingroombooker"
            remoteArtifacts = "/opt/meetingroombooker/backups/$Rollout/artifacts"
            remoteScripts = "/opt/meetingroombooker/backups/$Rollout/scripts"
        }
    }

    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $ManifestPath -Encoding UTF8

    $hashRows = @(
        [pscustomobject]@{ File = "api.zip"; SHA256 = Get-ArtifactHash -Path $ApiZipPath },
        [pscustomobject]@{ File = "web.zip"; SHA256 = Get-ArtifactHash -Path $WebZipPath }
    )

    if ($DeployMode -eq "migration") {
        $hashRows += [pscustomobject]@{ File = "efbundle-linux-x64"; SHA256 = Get-ArtifactHash -Path $EfBundlePath }
    }

    $hashRows += [pscustomobject]@{ File = "release-manifest.json"; SHA256 = Get-ArtifactHash -Path $ManifestPath }

    $hashRows | Format-Table -AutoSize
}

Write-Host ""
Write-Host "Build complete."
Write-Host "Rollout:   $Rollout"
Write-Host "Artifacts: $ArtifactsDir"
Write-Host ""
Write-Host "Next:"
Write-Host "  .\scripts\deploy\upload-prod.ps1 -Rollout $Rollout -DeployMode $DeployMode"

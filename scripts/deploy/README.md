# Safe Production Deployment Scripts

These scripts provide a guarded manual deployment path for MeetingRoomBooker production releases.
They are intentionally separate from GitHub Actions and do not change the existing CI/CD workflows.

The deployment target is the current single-node VPS layout:

- API service: `meetingroombooker-api.service`
- API path: `/opt/meetingroombooker/api`
- SQLite DB: `/opt/meetingroombooker/api/app.db`
- Web root: `/var/www/meetingroombooker`
- Rollout backups: `/opt/meetingroombooker/backups/<rollout>`
- Uploaded artifacts: `/opt/meetingroombooker/backups/<rollout>/artifacts`
- Uploaded scripts: `/opt/meetingroombooker/backups/<rollout>/scripts`

Do not commit secrets, tokens, passwords, real private host names, SSH keys, production database files, or environment-specific config.
Only `deploy-config.sample.json` belongs in the repository. Keep `deploy-config.local.json` local.

## Files

- `build-prod.ps1`
  - Runs local git state checks, `dotnet restore`, `dotnet build`, `dotnet test`, API publish, Web publish, zip creation, SHA-256 output, and `release-manifest.json` creation.
  - In `api-web` mode, creates only `api.zip`, `web.zip`, and `release-manifest.json`.
  - In `migration` mode, also includes `efbundle-linux-x64`.
  - If Windows cannot create the linux-x64 EF bundle, pass an existing Linux-built bundle with `-MigrationBundlePath`.
  - Does not connect to production.

- `upload-prod.ps1`
  - Uploads the local rollout artifacts and remote scripts to the VPS under `/opt/meetingroombooker/backups/<rollout>`.
  - Verifies uploaded artifact hashes with `sha256sum`.
  - Does not stop the API and does not modify the production database.

- `remote-preflight.sh`
  - Runs on the VPS with `BACKUP_DIR` and `DEPLOY_MODE`.
  - Backs up the production DB with `sqlite3 .backup`.
  - Verifies backup integrity.
  - In `migration` mode, applies the EF Core migration bundle only to a copied DB.
  - Does not apply migrations to the production DB.
  - Creates `preflight.ok` only after success.

- `deploy-prod.sh`
  - Runs on the VPS with `BACKUP_DIR` and `DEPLOY_MODE`.
  - Stops if `preflight.ok` is missing.
  - Extracts zip files with Python and normalizes `\` zip entry paths to `/`.
  - Requires explicit `STOP`, `APPLY`, and `DEPLOY` confirmations at the risky stages.
  - Copies the production `app.db` and `appsettings.json` into the new API directory before swapping API files.
  - Copies the production Web `appsettings.json` into the new Web directory before swapping Web files when it exists.
  - Starts the API and prints service, socket, curl, journal, and final DB integrity output.

- `rollback-prod.sh`
  - Runs on the VPS with `BACKUP_DIR`.
  - Restores `app.db.final-before-production-migration.bak`, `api.pre-deploy`, and `web.pre-deploy`.
  - Starts the API and prints service, curl, journal, and DB integrity output.

- `deploy-config.sample.json`
  - Sample non-secret upload configuration.

## Local Config

Create a local config from the sample:

```powershell
Copy-Item .\scripts\deploy\deploy-config.sample.json .\scripts\deploy\deploy-config.local.json
```

Edit `deploy-config.local.json` with the VPS SSH host, user, port, key path, and backup root.
Do not put tokens, passwords, Chatwork secrets, or private production values in the sample file.

The SSH user used by `upload-prod.ps1` must be able to create and write the rollout directory under `/opt/meetingroombooker/backups`.
The remote preflight, deploy, and rollback scripts should be run with `sudo` because they read and write `/opt`, `/var/www`, SQLite backups, and systemd.

## API/Web Deployment Without Migration

Use this path when there is no DB schema change and no EF Core migration to apply.
This is the normal path for API/Web-only updates.

1. Build and package locally:

   ```powershell
   .\scripts\deploy\build-prod.ps1 -DeployMode api-web
   ```

2. Upload the rollout:

   ```powershell
   .\scripts\deploy\upload-prod.ps1 -DeployMode api-web
   ```

3. Run the preflight command printed by `upload-prod.ps1`:

   ```bash
   sudo env BACKUP_DIR=/opt/meetingroombooker/backups/<rollout> DEPLOY_MODE=api-web bash /opt/meetingroombooker/backups/<rollout>/scripts/remote-preflight.sh
   ```

4. Run deploy only after preflight succeeds:

   ```bash
   sudo env BACKUP_DIR=/opt/meetingroombooker/backups/<rollout> DEPLOY_MODE=api-web bash /opt/meetingroombooker/backups/<rollout>/scripts/deploy-prod.sh
   ```

5. At the prompts, type:

   - `STOP` before the API is stopped.
   - `DEPLOY` before API/Web directories are swapped.

`api-web` mode does not create an EF bundle and does not run production DB migration.

## Deployment With Migration

Use this path only when the release intentionally includes a production DB schema change.
Review the migration and rollback impact before starting.

1. Build and package locally:

   ```powershell
   .\scripts\deploy\build-prod.ps1 -DeployMode migration
   ```

   If Windows cannot create the linux-x64 bundle because the SQLite native library `e_sqlite3` cannot be loaded, create `efbundle-linux-x64` on Linux, WSL, or GitHub Actions and pass it explicitly:

   ```powershell
   .\scripts\deploy\build-prod.ps1 -DeployMode migration -MigrationBundlePath .\path\to\efbundle-linux-x64
   ```

2. Upload the rollout:

   ```powershell
   .\scripts\deploy\upload-prod.ps1 -DeployMode migration
   ```

3. Run the preflight command printed by `upload-prod.ps1`:

   ```bash
   sudo env BACKUP_DIR=/opt/meetingroombooker/backups/<rollout> DEPLOY_MODE=migration bash /opt/meetingroombooker/backups/<rollout>/scripts/remote-preflight.sh
   ```

The preflight script backs up the production DB, copies that backup, applies the migration bundle to the copy, prints migration history, prints the `WorkScheduleEntries` schema, prints key table counts, and verifies integrity.
It does not modify the production DB.

4. Run deploy only after preflight succeeds:

   ```bash
   sudo env BACKUP_DIR=/opt/meetingroombooker/backups/<rollout> DEPLOY_MODE=migration bash /opt/meetingroombooker/backups/<rollout>/scripts/deploy-prod.sh
   ```

5. At the prompts, type:

   - `STOP` before the API is stopped.
   - `APPLY` before the migration bundle is applied to the production DB.
   - `DEPLOY` before API/Web directories are swapped.

## Rollback

Use rollback if deployment fails after `deploy-prod.sh` has created the final DB backup and pre-deploy directory backups.

```bash
sudo env BACKUP_DIR=/opt/meetingroombooker/backups/<rollout> bash /opt/meetingroombooker/backups/<rollout>/scripts/rollback-prod.sh
```

At the prompt, type `ROLLBACK`.

Rollback restores:

- `/opt/meetingroombooker/api/app.db` from `app.db.final-before-production-migration.bak`
- `/opt/meetingroombooker/api` from `api.pre-deploy`
- `/var/www/meetingroombooker` from `web.pre-deploy`

Restoring the DB can discard production data created after the final pre-deploy backup.
Treat DB rollback as a last resort.

## Safety Notes

- `build-prod.ps1` and `upload-prod.ps1` do not connect to or change production runtime state except uploading files into the rollout backup directory.
- `remote-preflight.sh` never applies migrations to the production DB.
- `deploy-prod.sh` refuses to run without `preflight.ok`.
- The production DB is backed up with `sqlite3 .backup`.
- API artifacts containing `*.db`, `*.db-shm`, or `*.db-wal` are rejected.
- The production `app.db` and `appsettings.json` are copied into the new API publish directory before the API path is swapped.
- If `/var/www/meetingroombooker/appsettings.json` exists, it is copied into the new Web publish directory before the Web root is swapped.
- Web deployment uses the contents of the Blazor publish `wwwroot`.
- Zip extraction on the VPS uses Python and normalizes Windows `\` zip paths to `/`.
- These scripts do not modify nginx configuration, systemd unit files, GitHub Actions workflows, appsettings secrets, migrations, or application code.

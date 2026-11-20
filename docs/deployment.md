# Deployment Workflow

This document describes the deployment architecture and release workflow for MeetingRoomBooker.

MeetingRoomBooker is operated as a small internal production system. The deployment process is designed to keep production updates explicit, reviewable, and safe, especially because the system uses a real production database.

---

## Architecture Overview

MeetingRoomBooker is deployed as a single-node internal web application.

Architecture:

    Browser
      |
      | HTTPS
      v
    Nginx
      |
      |-- Serves Blazor WebAssembly static files
      |
      |-- Proxies /api requests to ASP.NET Core Web API
              |
              v
          Kestrel API process
              |
              v
            SQLite

Runtime components:

- Blazor WebAssembly
  - Published as static files
  - Served by Nginx

- ASP.NET Core Web API
  - Published as a .NET application
  - Runs as a long-running Kestrel process
  - Managed by systemd

- Nginx
  - Serves the Web frontend
  - Proxies API requests to the local API process

- SQLite
  - Used as the production database
  - Kept separate from publish artifacts

- Chatwork notifications
  - Handled by the backend
  - API tokens are not exposed to the frontend

---

## Publish Outputs

The API and Web projects are published separately.

API publish output:

    publish/api/

Expected API file:

    publish/api/MeetingRoomBooker.Api.dll

Web publish output:

    publish/web/wwwroot/

Expected Web files:

    publish/web/wwwroot/index.html
    publish/web/wwwroot/_framework/
    publish/web/wwwroot/MeetingRoomBooker.Web.styles.css

The Web deployment uses the contents of publish/web/wwwroot, not the publish/web directory itself.

---

## GitHub Actions Workflows

Automatic CI validates changes in GitHub Actions. Production deployment requires a manual workflow dispatch and explicit confirmation.

| Workflow | Trigger | Role |
| --- | --- | --- |
| `dotnet-ci` | Pull requests targeting `master` and pushes to `master` | Restores dependencies, builds in Release mode, runs xUnit tests, publishes the API and Web, rejects database files in the outputs, and uploads verified outputs as GitHub Actions artifacts. |
| `operations-web-ci` | Pull requests targeting `master` and pushes to `master`. Both events require changes under `MeetingRoomBooker.OperationsWeb/**` or to `.github/workflows/operations-web-ci.yml`. | Installs dependencies with `npm ci` and builds OperationsWeb. |
| `manual-vps-deploy` | Manual dispatch (`workflow_dispatch`) with deployment and no-schema-change confirmations | Builds, tests, and deploys the API and Web to production for releases without database schema changes. |
| `manual-vps-deploy-with-migration` | Manual dispatch (`workflow_dispatch`) with deployment, schema-change, and backup confirmations | Builds and tests the release, backs up SQLite, applies the EF Core migration bundle, and deploys the API and Web to production. |

---

## Manual Production Deployment

Production deployment is intentionally manual.

The production deployment workflow requires explicit confirmation before running.

Required inputs:

    confirm_deploy = DEPLOY
    confirm_no_db_schema_change = NO_DB_CHANGE

The workflow is intended only for deployments that do not require database schema changes.

The workflow:

1. validates manual confirmation inputs
2. restores dependencies
3. builds the solution in Release configuration
4. runs xUnit tests
5. publishes the API project
6. publishes the Web project
7. verifies publish outputs
8. checks that database files are not included in publish outputs
9. uploads API output to the VPS deploy directory
10. uploads Web wwwroot output to the VPS deploy directory
11. runs the existing VPS deployment script

---

## VPS Deployment Script

The VPS deployment script is responsible for applying the uploaded files to the production runtime directories.

It performs the following operations:

1. backs up the current API directory
2. backs up the current Web directory
3. backs up the production SQLite database
4. syncs API publish output to the API runtime directory
5. syncs Web publish output to the Web runtime directory
6. restarts the API systemd service
7. validates the Nginx configuration
8. reloads Nginx
9. runs basic health checks

The script excludes the production database from API file synchronization so that publish artifacts do not overwrite the live SQLite database.

---

## Database Policy

The manual production deployment workflow must only be used when there are no database schema changes.

Examples of database schema changes:

- adding a column
- removing a column
- renaming a column
- adding a table
- changing indexes
- adding or modifying EF Core migrations

For deployments with database schema changes, use a separate migration procedure.

A database-change release should include:

1. production database backup
2. migration plan
3. migration execution steps
4. verification SQL
5. application deployment
6. rollback plan

The normal deployment workflow should not be used blindly for schema-changing releases.

---

## Schema-changing Production Deployment

Deployments that include database schema changes must not use the normal `manual-vps-deploy` workflow directly.

Schema-changing changes include:

- adding a table
- adding, removing, or renaming a column
- changing indexes
- adding or modifying EF Core migrations
- changing entity mappings that affect the database schema

For schema-changing deployments, use the dedicated manual workflow:

```text
manual-vps-deploy-with-migration
```

This workflow is intentionally manual and requires explicit confirmation inputs.

Required confirmations:

```text
confirm_deploy = DEPLOY
confirm_schema_change = APPLY_MIGRATION
confirm_backup = BACKUP_DB
```

The schema-changing deployment flow is:

1. restore dependencies
2. build the solution in Release configuration
3. run tests
4. publish the API project
5. publish the Web project
6. build an EF Core migration bundle
7. verify publish outputs
8. verify that database files are not included in publish outputs
9. upload API, Web, and migration bundle files to the VPS deploy directories
10. stop the API systemd service
11. back up the production SQLite database
12. run the migration bundle against the production database
13. run the VPS deployment script
14. verify the application after deployment

The production database path must be stored as a GitHub Actions secret.

Required secret:

```text
MRB_PRODUCTION_DB_PATH
```

Example value:

```text
/opt/meetingroombooker/api/app.db
```

Do not commit the production database path, production URL, SSH keys, API tokens, or any real database files.

### Why migration bundles are used

EF Core migration bundles create a single executable that applies pending migrations to the target database. This makes database migration execution explicit and easier to include in a deployment process than running migrations automatically when the application starts.

Application startup should not automatically apply production migrations in this project. Production database changes should be backed up, reviewed, and executed intentionally.

### Production database path

The current production database path must be confirmed before running the schema-changing deployment workflow.

Possible database files may exist in old backup directories. Do not choose a path only because `app.db` exists there.

Confirm the active database path by checking:

- the systemd service configuration
- the API working directory
- the production connection string
- the current runtime directory used by the deployment script

Useful VPS commands:

```bash
systemctl cat meetingroombooker-api.service
systemctl status meetingroombooker-api.service
grep -R "Data Source" /opt/meetingroombooker/api /var/www/api /etc/systemd/system 2>/dev/null
```

The `MRB_PRODUCTION_DB_PATH` secret should point to the active production database only.

### Rollback notes for schema-changing deployments

Before applying a migration, the workflow backs up the production SQLite database.

If the application deployment fails after the migration is applied, first check whether the application can run with the migrated schema. For additive migrations such as adding a new table, the previous application version will usually continue to work.

Restoring a database backup should be treated as a last resort because it can remove reservations or operational data created after the backup.

---

## Safety Checks

The deployment process includes several safety checks:

- CI runs build and tests before deployment
- publish outputs are verified before upload
- database files are rejected from publish outputs
- deployment is manually triggered
- production deployment requires explicit confirmation inputs
- the VPS script backs up API, Web, and database files before applying changes
- Nginx configuration is tested before reload

---

## Rollback Policy

The VPS deployment script creates timestamped backups of the previous API and Web directories.

If an application deployment needs to be rolled back, restore the previous API and Web backup directories and restart the API service.

Database rollback should be treated as a last resort because restoring a database backup can also remove reservations or operational data created after the backup.

---

## Production Verification Checklist

After deployment, verify:

- the API service is running
- Nginx reload succeeded
- the login page opens
- existing reservations are visible
- calendar and timeline views load
- reservation creation screen opens
- admin screen opens
- notification screen opens
- application logs do not contain database schema errors such as no such column
- application logs do not contain unhandled exceptions

---

## Notes

- Secrets such as SSH keys, tokens, passwords, and production URLs must not be committed.
- Production database files must not be included in publish outputs.
- Deployment automation is intentionally manual to reduce the risk of accidental production updates.

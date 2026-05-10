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

## CI Workflow

The main CI workflow runs on pull requests and pushes to master.

It checks:

- dependency restore
- Release build
- xUnit tests
- API publish
- Web publish
- expected publish output files
- database files are not included in publish outputs
- API and Web publish outputs can be uploaded as GitHub Actions artifacts

This helps catch build, test, and publish issues before deployment.

---

## Manual Publish Workflow

A separate manual workflow can be used to generate publish artifacts on demand.

This workflow:

- runs restore, build, and test
- publishes the API project
- publishes the Web project
- verifies required publish outputs
- uploads API and Web outputs as artifacts

This workflow does not deploy to the VPS.

---

## Manual VPS Connection Check

A manual VPS connection check workflow verifies that GitHub Actions can connect to the VPS through SSH.

This workflow checks:

- SSH connectivity
- expected runtime directories
- the presence of the deployment script

This workflow does not upload files and does not deploy the application.

---

## Manual VPS Upload Checks

Before enabling production deployment, upload checks are separated into safe stages.

### Temporary upload check

A workflow uploads publish outputs to a temporary directory on the VPS.

Example layout:

    /root/mrb-artifact-upload-check/api
    /root/mrb-artifact-upload-check/web

This confirms that GitHub Actions can upload API and Web publish outputs to the VPS without touching production deployment directories.

### Deploy directory upload check

A workflow uploads publish outputs to deploy staging directories.

Example layout:

    /root/deploy/api
    /root/deploy/web

This prepares the files that the deployment script will later use.

This workflow does not run the deployment script.

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

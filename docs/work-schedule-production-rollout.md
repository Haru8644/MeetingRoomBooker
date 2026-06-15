# Work Schedule Production Rollout Plan

This document describes the production rollout procedure for the work schedule and notification updates in MeetingRoomBooker.

The rollout includes database schema changes. Do not apply migrations directly to production without confirming the actual production database path, current migration history, backup status, and rollback procedure.

This document is intentionally written as an operational checklist. Before running production migrations, collect the current VPS state and verify it against this checklist.

---

## Scope

This rollout covers the production deployment of the work schedule feature set and its notification support.

Included areas:

* work schedule entries
* external appointments
* work-from-home schedules
* leave schedules
* participant conflict warnings
* in-app notifications for work schedules
* Chatwork notifications for work schedules
* reservation Chatwork warnings when participants conflict with external appointments

---

## Related migrations

The following migrations are related to the recent work schedule updates.

```text
20260613084412_AddWorkScheduleEntries
20260615083231_AddWorkScheduleNotificationTarget
20260615083733_AddWorkScheduleChatworkDeliveryLogTarget
```

These migrations may not all be unapplied in production. Always verify the actual production migration state before applying anything.

---

## Main risk

Production and local database schemas may not be perfectly aligned.

A previous local migration attempt showed an error similar to:

```text
SQLite Error 1: 'table "ChatworkDeliveryLogs" already exists'.
```

This can happen when a table already exists while `__EFMigrationsHistory` does not correctly reflect the actual schema, or when the migration command is pointing to a different database than expected.

Because production uses SQLite, a mistaken migration can directly modify the live database file. Always confirm the actual database path and create backups before running any schema-changing command.

---

## Production assumptions

Typical production layout:

```text
API systemd service:
meetingroombooker-api.service

API listen address:
127.0.0.1:5000

Static web root:
/var/www/meetingroombooker

Database:
SQLite app.db
```

These are assumptions only. The actual production paths must be confirmed before deployment.

Do not write real VPS paths, private hostnames, Chatwork room IDs, or secrets into this document.

---

## Step 1: Collect current VPS state

Before changing anything, collect read-only information from the VPS.

```bash
sudo systemctl cat meetingroombooker-api.service
sudo systemctl status meetingroombooker-api.service --no-pager
sudo journalctl -u meetingroombooker-api.service -n 100 --no-pager
```

Find candidate database and configuration files.

```bash
find / -name "app.db" 2>/dev/null
find / -name "appsettings*.json" 2>/dev/null
```

Check nginx configuration if needed.

```bash
sudo nginx -T 2>/dev/null | grep -E "server_name|root|proxy_pass" -n
```

Record the confirmed values locally before deployment:

```text
Production DB path:
API working directory:
API service name:
Static web root:
API listen address:
```

Do not continue until the actual production database path is known.

---

## Step 2: Create a timestamped database backup

After confirming the database path, create a backup.

```bash
DB_PATH="/path/to/app.db"
BACKUP_PATH="${DB_PATH}.$(date +%Y%m%d_%H%M%S).bak"

cp "$DB_PATH" "$BACKUP_PATH"
ls -lh "$DB_PATH" "$BACKUP_PATH"
```

Verify that the backup file exists and has a reasonable size.

---

## Step 3: Inspect current migration history

Use SQLite to inspect `__EFMigrationsHistory`.

```bash
sqlite3 "$DB_PATH" ".tables"
sqlite3 "$DB_PATH" "SELECT MigrationId, ProductVersion FROM __EFMigrationsHistory ORDER BY MigrationId;"
```

Confirm whether these migrations are already applied:

```text
20260613084412_AddWorkScheduleEntries
20260615083231_AddWorkScheduleNotificationTarget
20260615083733_AddWorkScheduleChatworkDeliveryLogTarget
```

If `__EFMigrationsHistory` does not exist or appears incomplete, stop and inspect the actual table schemas before applying migrations.

---

## Step 4: Inspect current table list

Check whether expected tables already exist.

```bash
sqlite3 "$DB_PATH" ".tables"
```

Important tables:

```text
Users
Reservations
Notifications
ChatworkDeliveryLogs
RoomConflictRecords
WorkScheduleEntries
__EFMigrationsHistory
```

Stop and investigate if:

* `WorkScheduleEntries` already exists but its migration is not recorded
* `ChatworkDeliveryLogs` exists but the migration history looks incomplete
* `Notifications` already has newer columns but the migration history does not reflect them

Do not blindly run `dotnet ef database update` in these cases.

---

## Step 5: Inspect relevant table schemas

```bash
sqlite3 "$DB_PATH" ".schema WorkScheduleEntries"
sqlite3 "$DB_PATH" ".schema Notifications"
sqlite3 "$DB_PATH" ".schema ChatworkDeliveryLogs"
sqlite3 "$DB_PATH" ".schema Reservations"
```

Expected recent schema changes include:

* `WorkScheduleEntries` table
* `Notifications.TargetWorkScheduleEntryId`
* `ChatworkDeliveryLogs.WorkScheduleEntryId`
* indexes related to notifications and Chatwork delivery logs

---

## Step 6: Test migration on a copied database when possible

Create a copied database and test migrations against that copy first.

```bash
DB_PATH="/path/to/app.db"
TEST_DB_PATH="/tmp/meetingroombooker-migration-test-$(date +%Y%m%d_%H%M%S).db"

cp "$DB_PATH" "$TEST_DB_PATH"
ls -lh "$TEST_DB_PATH"
```

Point the migration command to the copied database using a temporary configuration or environment-specific connection string.

Do not test against the live database.

After migration testing, inspect:

```bash
sqlite3 "$TEST_DB_PATH" "SELECT MigrationId FROM __EFMigrationsHistory ORDER BY MigrationId;"
sqlite3 "$TEST_DB_PATH" ".schema WorkScheduleEntries"
sqlite3 "$TEST_DB_PATH" ".schema Notifications"
sqlite3 "$TEST_DB_PATH" ".schema ChatworkDeliveryLogs"
```

If the copied database migration fails, do not continue to production.

---

## Step 7: Pull the latest code on the server

```bash
cd /path/to/MeetingRoomBooker
git fetch origin
git switch master
git pull --ff-only origin master
git log --oneline -5
```

Confirm that the latest target merge commit is present.

---

## Step 8: Build and test on the server

```bash
dotnet build
dotnet test MeetingRoomBooker.slnx
```

If build or tests fail on the server, stop and investigate before continuing.

---

## Step 9: Stop the API

```bash
sudo systemctl stop meetingroombooker-api.service
sudo systemctl status meetingroombooker-api.service --no-pager
```

Confirm that the API has stopped before applying migrations.

---

## Step 10: Create a second backup immediately before migration

```bash
DB_PATH="/path/to/app.db"
BACKUP_PATH="${DB_PATH}.before-work-schedule-rollout-$(date +%Y%m%d_%H%M%S).bak"

cp "$DB_PATH" "$BACKUP_PATH"
ls -lh "$DB_PATH" "$BACKUP_PATH"
```

Keep this backup until the rollout has been fully verified.

---

## Step 11: Apply migrations

Only run this after:

* the DB path is confirmed
* backups exist
* migration history has been inspected
* table schemas have been inspected
* copied database migration has been tested when possible
* the API has been stopped

Example:

```bash
dotnet ef database update --project MeetingRoomBooker.Api --startup-project MeetingRoomBooker.Api
```

After applying migrations, verify:

```bash
sqlite3 "$DB_PATH" "SELECT MigrationId FROM __EFMigrationsHistory ORDER BY MigrationId;"
sqlite3 "$DB_PATH" ".schema WorkScheduleEntries"
sqlite3 "$DB_PATH" ".schema Notifications"
sqlite3 "$DB_PATH" ".schema ChatworkDeliveryLogs"
```

---

## Step 12: Publish API and Web

Example publish commands:

```bash
dotnet publish MeetingRoomBooker.Api -c Release -o ./publish/api
dotnet publish MeetingRoomBooker.Web -c Release -o ./publish/web
```

If production deployment uses GitHub Actions or a manual publish workflow, follow that workflow instead of ad-hoc copying.

Confirm the actual production paths before replacing files.

---

## Step 13: Deploy static Web files

If deploying manually:

```bash
sudo rsync -av --delete ./publish/web/wwwroot/ /var/www/meetingroombooker/
```

Confirm that `/var/www/meetingroombooker` is the actual static web root before running `rsync --delete`.

---

## Step 14: Restart the API

```bash
sudo systemctl start meetingroombooker-api.service
sudo systemctl status meetingroombooker-api.service --no-pager
sudo journalctl -u meetingroombooker-api.service -n 100 --no-pager
```

If the service fails to start, do not continue. Check logs first.

---

## Post-deployment verification

Verify the following in production.

### Basic access

* Login works
* Schedule page opens
* Reservation page opens
* Notification page opens
* Admin/user management page opens if applicable

### Meeting room reservations

* Existing reservations are visible
* New reservation can be created
* Reservation can be edited
* Reservation can be deleted
* Room/time conflict validation still works
* Participant conflict warnings still appear

### Work schedules

* External appointment can be created
* Work-from-home schedule can be created
* Leave schedule can be created
* Work schedules appear on the schedule page
* External appointments appear in the timeline
* Work-from-home and leave schedules do not overcrowd the timeline
* Work schedule detail modal opens
* Work schedule edit works
* Work schedule delete works

### In-app notifications

* Work schedule creation creates in-app notifications
* Work schedule update creates in-app notifications
* Work schedule deletion creates in-app notifications
* Participant conflict warnings appear in notifications
* Existing reservation notifications still work

### Chatwork

* Work schedule creation sends Chatwork direct notifications
* Work schedule update sends Chatwork direct notifications
* Work schedule deletion sends Chatwork direct notifications
* Users without `ChatworkDirectRoomId` are skipped without breaking other deliveries
* `ChatworkDeliveryLogs` records succeeded, failed, or skipped deliveries
* Reservation Chatwork messages include participant conflict warnings when overlapping external appointments exist
* Existing reservation reminders still work

### Logs

Check API logs after verification.

```bash
sudo journalctl -u meetingroombooker-api.service -n 200 --no-pager
```

Confirm that there are no repeated migration, database, or Chatwork errors.

---

## Rollback plan

### If deployment fails before migration

1. stop the API if needed
2. restore previous application files if needed
3. start the API
4. verify core workflows

### If deployment fails after migration

Stop the API:

```bash
sudo systemctl stop meetingroombooker-api.service
```

Restore the database backup:

```bash
DB_PATH="/path/to/app.db"
BACKUP_PATH="/path/to/app.db.before-work-schedule-rollout-YYYYMMDD_HHMMSS.bak"

cp "$BACKUP_PATH" "$DB_PATH"
```

Restore previous application files if necessary.

Start the API:

```bash
sudo systemctl start meetingroombooker-api.service
sudo systemctl status meetingroombooker-api.service --no-pager
```

Verify core workflows.

Do not partially roll back only the application or only the database unless the compatibility impact is fully understood.

---

## Do not do these

Do not:

* run `dotnet ef database update` before confirming the production DB path
* assume local and production migration histories are the same
* assume the database file is always named `app.db` without checking
* run destructive `rsync --delete` before confirming the target directory
* deploy without a timestamped SQLite backup
* ignore `__EFMigrationsHistory`
* ignore existing table schemas
* apply migrations while the API is running
* commit production database files
* commit real Chatwork tokens
* continue deployment if copied database migration fails
* write real production secrets or private server values into this document

---

## Production rollout checklist

Before production deployment, confirm:

```text
[ ] Current VPS state collected
[ ] Production DB path confirmed
[ ] API service name confirmed
[ ] API working directory confirmed
[ ] Static web root confirmed
[ ] First DB backup created
[ ] __EFMigrationsHistory inspected
[ ] Table schemas inspected
[ ] Copied DB migration tested when possible
[ ] Latest master pulled on server
[ ] Server-side build passed
[ ] Server-side tests passed
[ ] API stopped
[ ] Second DB backup created
[ ] Migrations applied
[ ] Migration history verified after update
[ ] API restarted
[ ] Web deployed
[ ] Basic access verified
[ ] Reservation workflows verified
[ ] Work schedule workflows verified
[ ] In-app notifications verified
[ ] Chatwork notifications verified
[ ] API logs checked
[ ] Rollback backup retained
```

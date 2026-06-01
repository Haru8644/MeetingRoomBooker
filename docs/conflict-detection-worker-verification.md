# Room Conflict Detection Worker Verification

## Purpose

This document describes how to verify the room conflict detection worker locally before enabling it in production.

The worker detects reservation overlaps that remain unresolved when the actual overlapping time starts. It records them as `UnresolvedReservationOverlap` with `Detected` status in `RoomConflictRecords`.

The production configuration keeps the worker disabled by default.

## Safety Principles

* Do not enable the worker in production until the database migration has been applied.
* Do not test against the production SQLite database.
* Use a temporary local SQLite database for verification.
* Enable the worker through environment variables only during local verification.
* Keep `RoomConflictDetection:Enabled` set to `false` in committed configuration files.
* Do not send Chatwork notifications from this worker.

## Local Verification Summary

The local verification confirms the following:

1. A temporary SQLite database can be created.
2. EF Core migrations can be applied successfully.
3. The API starts with the worker enabled locally.
4. Overlapping reservations can be inserted into the temporary database.
5. The worker creates one `RoomConflictRecords` entry.
6. Running the worker again does not create a duplicate record.

## Temporary Database Setup

From the repository root:

```powershell
$dbPath = Join-Path (Resolve-Path MeetingRoomBooker.Api).Path "conflict-detection-test.db"

if ($dbPath -and $dbPath.EndsWith("conflict-detection-test.db")) {
    Remove-Item "$dbPath*" -ErrorAction SilentlyContinue
}

$env:ASPNETCORE_ENVIRONMENT = "Development"
$env:ConnectionStrings__DefaultConnection = "Data Source=$dbPath"
$env:Database__EnsureCreatedOnStartup = "false"
$env:Database__ApplyMigrationsOnStartup = "true"
$env:RoomConflictDetection__Enabled = "true"
$env:RoomConflictDetection__IntervalMinutes = "1"
$env:RoomConflictDetection__LookbackMinutes = "15"
```

Then start the API:

```powershell
dotnet run --project MeetingRoomBooker.Api
```

Stop the API after it starts successfully.

Expected result:

* The API starts successfully.
* EF Core applies migrations to the temporary database.
* The `RoomConflictRecords` table is created.
* No startup error occurs.

## Seed Tool

Create a temporary console project outside the repository:

```powershell
$seedDir = Join-Path $env:TEMP "mrb-conflict-seed"

Remove-Item $seedDir -Recurse -Force -ErrorAction SilentlyContinue

dotnet new console -o $seedDir --framework net8.0
dotnet add $seedDir package Microsoft.Data.Sqlite --version 8.*
notepad (Join-Path $seedDir "Program.cs")
```

Replace `Program.cs` with a small SQLite seed tool that can:

* delete existing `RoomConflictRecords`
* delete existing `Reservations`
* insert two overlapping reservations in the same room
* display rows from `RoomConflictRecords`

The seed tool must not be committed to the repository.

## Verification Steps

### 1. Insert overlapping reservations

```powershell
dotnet run --project $seedDir -- "$dbPath" seed
```

Expected output:

```text
Seeded overlapping reservations.
Overlap start: <timestamp>
```

### 2. Start the API with the worker enabled

```powershell
dotnet run --project MeetingRoomBooker.Api
```

Wait for the worker to run, then stop the API.

Expected log:

```text
Created 1 unresolved reservation overlap records.
Created 1 room conflict records for unresolved reservation overlaps.
```

### 3. Confirm that one detected record was created

```powershell
dotnet run --project $seedDir -- "$dbPath" show
```

Expected output:

```text
Id=1, Type=0, Status=0, OccurredAt=<timestamp>, Room=<room>, A=1, B=2, Key=<detection-key>
RoomConflictRecords count: 1
```

Meaning:

* `Type=0` means `UnresolvedReservationOverlap`.
* `Status=0` means `Detected`.
* `A=1, B=2` means the detected record is linked to the two overlapping reservations.
* `RoomConflictRecords count: 1` means one conflict record was created.

### 4. Confirm duplicate prevention

Start the API again:

```powershell
dotnet run --project MeetingRoomBooker.Api
```

Stop it after the worker runs.

Then check the records again:

```powershell
dotnet run --project $seedDir -- "$dbPath" show
```

Expected output:

```text
RoomConflictRecords count: 1
```

If the count remains `1`, duplicate prevention by `DetectionKey` is working.

## Cleanup

Remove the temporary environment variables:

```powershell
Remove-Item Env:ASPNETCORE_ENVIRONMENT -ErrorAction SilentlyContinue
Remove-Item Env:ConnectionStrings__DefaultConnection -ErrorAction SilentlyContinue
Remove-Item Env:Database__EnsureCreatedOnStartup -ErrorAction SilentlyContinue
Remove-Item Env:Database__ApplyMigrationsOnStartup -ErrorAction SilentlyContinue
Remove-Item Env:RoomConflictDetection__Enabled -ErrorAction SilentlyContinue
Remove-Item Env:RoomConflictDetection__IntervalMinutes -ErrorAction SilentlyContinue
Remove-Item Env:RoomConflictDetection__LookbackMinutes -ErrorAction SilentlyContinue
```

Remove the temporary database safely:

```powershell
if ($dbPath -and $dbPath.EndsWith("conflict-detection-test.db")) {
    Remove-Item "$dbPath*" -ErrorAction SilentlyContinue
}
```

Remove the temporary seed tool:

```powershell
Remove-Item $seedDir -Recurse -Force -ErrorAction SilentlyContinue
```

## Verified Behavior

Local verification confirmed that:

* migrations are applied to the temporary SQLite database,
* the API starts successfully with the worker enabled locally,
* the worker creates one detected room conflict record for unresolved overlapping reservations,
* the created record uses `UnresolvedReservationOverlap` and `Detected`,
* duplicate records are not created on subsequent worker runs.

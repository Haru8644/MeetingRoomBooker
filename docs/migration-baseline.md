# MeetingRoomBooker migration baseline

For [issue #80](https://github.com/Haru8644/MeetingRoomBooker/issues/80), use `d8fe3b7` as the reference when comparing the Go API and React UI with the current application.

## Current application

Paths below are relative to the repository root. Blazor WebAssembly registers `ApiBookingService` and `WorkScheduleApiService` in `MeetingRoomBooker.Web/Program.cs`. Each API row gives the base path followed by its methods and relative paths. A method alone acts on the base path.

| Feature and screen | HTTP surface | Behavior to preserve |
| --- | --- | --- |
| Reservations on `/reservation` and `/schedule` | `/api/reservations` — GET, POST, POST `series`, PUT/DELETE `{id}`, POST `{id}/join`, `{id}/leave`, `{id}/series-update`, `{id}/series-delete` | An authenticated user lists bookings and creates them as the current organizer. Creation returns 409 on a room conflict unless `allowOverlap=true`. Single and series edits can save an overlap. The API authorizes organizer or admin to edit/delete. The calendar shows manage controls to the organizer. Join/leave changes participants and can notify the organizer. Recurrence skips weekends and supports daily, weekly and biweekly bookings. Preserve legacy series lookup when `SeriesId` is absent. |
| Work schedules on the same two pages | `/api/work-schedule-entries` — GET (optional inclusive `from`, `to`), GET/PUT/DELETE `{id}`, POST, POST `series`, PUT/DELETE `{id}/series` | Timed external appointments, work from home, and morning/afternoon/full-day leave appear alongside bookings. Only external appointments support recurrence (daily/weekly). The API allows creator or admin to manage entries. The UI shows edit/delete to the creator. Bulk external edits keep occurrence dates. Shared participants can trigger warnings across appointments and bookings. |
| Home `/`, login `/login`, navigation, settings `/settings` | `/api/auth` — POST `login`, GET `me`, POST `logout` | Login normalizes email, verifies the ASP.NET password hash and upgrades a matching legacy password. The `MeetingRoomBooker.Auth` cookie supports a session or persistent 14-day sign-in and sliding expiration. Protected endpoints return 401/403. Home and navigation reflect login, admin and unread state. Settings toggles the dark/light theme. |
| User management `/users` and participant pickers | `/api/users` — GET, GET `participants`, POST `register`, PUT `{id}/name`, `{id}/chatwork-account`, `{id}/chatwork-direct-room`, DELETE `{id}` | Admin console lists, registers, renames, configures and deletes users. Renaming updates owned booking names. Authenticated pickers get ID, name and avatar color. The first registered user becomes admin. Registration has its own existing-user guard. Verify its role behavior independently of console visibility. |
| Notification center `/notifications` and nav badge | `/api/notifications` — GET `user/{userId}`, POST, PUT `{id}/read`, DELETE `{id}`. Admin-only POST `/api/chatwork/test` | Recipient or admin can list, mark read and delete. Only admins can POST a notification. Bulk UI actions iterate individual IDs. Opening a notice marks it read and navigates to `/schedule?BookingId=…` when it has a reservation target. Work-schedule targets also exist in storage. Booking and schedule changes can write in-app notices and invoke Chatwork delivery to individuals. |
| React OperationsWeb conflict dashboard | `/api/room-conflict-records` — GET, GET `{id}`, GET `summary`, POST, PUT `{id}` | Authenticated users see summary cards and a record list, submit a manual collision and select a record for review. Reporter or admin can update its classification/resolution. Keep the existing client, its credentialed requests, edit permissions and refresh after save. |

### Source files

| Area | Repository paths |
| --- | --- |
| Bookings and schedules | `MeetingRoomBooker.Web/Pages/Reservation.razor`, `MeetingRoomBooker.Web/Pages/Schedule.razor`, `MeetingRoomBooker.Api/Controllers/ReservationsController.cs`, `MeetingRoomBooker.Api/Controllers/WorkScheduleEntriesController.cs`, `MeetingRoomBooker.Api/Services/Reservations/ReservationSeriesQueryService.cs`, `MeetingRoomBooker.Api/Services/WorkSchedules/WorkScheduleEntryService.cs` |
| Identity, settings and users | `MeetingRoomBooker.Web/Pages/Index.razor`, `MeetingRoomBooker.Web/Pages/Login.razor`, `MeetingRoomBooker.Web/Pages/Settings.razor`, `MeetingRoomBooker.Web/Pages/UserManagement.razor`, `MeetingRoomBooker.Web/Layout/NavMenu.razor`, `MeetingRoomBooker.Api/Program.cs`, `MeetingRoomBooker.Api/Controllers/AuthController.cs`, `MeetingRoomBooker.Api/Controllers/UsersController.cs` |
| Notifications | `MeetingRoomBooker.Web/Pages/Notification.razor`, `MeetingRoomBooker.Api/Controllers/NotificationsController.cs`, `MeetingRoomBooker.Api/Controllers/ChatworkController.cs`, `MeetingRoomBooker.Api/Services/Chatwork/ReservationChatworkNotificationService.cs`, `MeetingRoomBooker.Api/Services/Chatwork/WorkScheduleChatworkNotificationService.cs` |
| Operations | `MeetingRoomBooker.OperationsWeb/src/App.tsx`, `MeetingRoomBooker.OperationsWeb/src/features/roomConflicts/roomConflictApi.ts`, `MeetingRoomBooker.Api/Controllers/RoomConflictRecordsController.cs`, `MeetingRoomBooker.Api/Services/RoomConflictRecords/RoomConflictRecordService.cs` |

### Background jobs

- `MeetingRoomBooker.Api/Services/Chatwork/ChatworkReminderWorker.cs` polls every minute when running. With Chatwork enabled, it selects reservations and timed external appointments whose start falls in `(now, now + 10 minutes]` and invokes per-recipient reminder delivery.
- `MeetingRoomBooker.Api/Services/RoomConflictRecords/RoomConflictDetectionWorker.cs` runs detection when enabled. The option defaults are a one-minute interval and a 15-minute lookback. `MeetingRoomBooker.Api/Services/RoomConflictRecords/RoomConflictDetectionService.cs` inserts a Detected record when an overlap begins within that window. A key derived from room, date, ordered booking IDs and overlap interval suppresses repeat detection.

**Production confirmation required.** Confirm deployed revisions and routes (including OperationsWeb), origins/cookie settings, time zone, effective worker flags/intervals and Chatwork recipient configuration with the owner. Confirm SQLite schema/migration history, legacy data shapes, volume, backup availability and recovery window before sizing the move. Repository defaults and sample screenshots cannot establish these facts.

## Technology decisions and checks

| Decision | Reason, alternative and migration cost | Verification |
| --- | --- | --- |
| Go API and workers | I chose Go because I want to use it for future backend work. I'll start with the reservation overlap check so I can compare its output directly with C# before moving authentication or storage. Continuing ASP.NET Core retains working controllers, Identity, EF queries and hosted services at the lowest porting cost. Go requires equivalents for cookie/role checks, hash compatibility, date handling, transactions, job cancellation and delivery. | Run identical synthetic inputs against both backends. Compare status, JSON, resulting rows and fake sends for anonymous, member, owner and admin. Start with the predicate below. |
| React and TypeScript for the main UI | The existing OperationsWeb supplies a typed, credentialed API client and a React toolchain. Reuse that app and its conflict screens. Keeping Blazor preserves Fluent components and current rendering. Rebuilding the main pages costs component, CSS, browser-storage, navigation and dialog parity work. | Run the capture matrix and interaction checks below. Run the existing OperationsWeb list, summary, report and review flows against each API with the same cookie roles. |
| PostgreSQL | I chose PostgreSQL to support concurrent writes from the API and background workers. I'll test what happens when two users try to book the same room at the same time while a worker is writing to the database. Keeping SQLite retains the deployed file format, EF conversions and simple backup procedure. PostgreSQL adds service operations, schema/type mapping, ID sequences and a reverse conversion to SQLite. | Round-trip synthetic data, exercise concurrent booking and worker writes, and rehearse restore. Measure latency, throughput and lock waits under the same workload before making performance claims. |
| Go HTTP bootstrap | Use `net/http` for local probes to keep routing and timeouts in the standard library. A web framework would provide shared middleware and routing conventions at the cost of an extra dependency. | Exercise the probe routes with `httptest` and check the local server with `curl`. |
| Go PostgreSQL connection pool | Use `pgxpool` for readiness checks and connection reuse. A direct connection per probe would simplify pool lifecycle management and require a new handshake on every check. The pool requires closing on shutdown and lets the API start while PostgreSQL is unavailable. | Substitute a ping function in unit tests and check readiness with a local PostgreSQL instance running and stopped. |

## Blazor visual and interaction baseline

Proposed capture settings are 1440×900 and 390×844 CSS pixels, DPR 1, 100% zoom, a pinned browser/OS and Japanese fonts, `ja-JP`, `Asia/Tokyo`, and a fixed clock (2026-06-01 09:00). Seed identical synthetic users, roles, stable IDs, appointments and delivery history. Reset session/draft state between scenarios. Fix scroll, focus, theme and animation time, and wait for fonts and data. Save each reference PNG with its route, actions, fixture and rendering settings. Compare equal-size captures at zero tolerance across all pixels, with **0 differing pixels** required per state and viewport. For browser-native alerts/confirms, capture the same browser window and assert the dialog text and both action outcomes where applicable.

| Route | Capture points and actions |
| --- | --- |
| `/`, `/login`, shared navigation | Guest and signed-in home actions. Empty form, missing/invalid credentials, remember-me off/on, successful redirect to schedule. Member/admin links, unread badge, mobile profile open/closed, logout. |
| `/reservation` | Blank and filled internal/visitor booking, room and participant selections, recurrence end date, validation, empty/populated day preview. Visit schedule and restore the draft. Capture external appointment single/recurring, work-from-home and each leave period. Trigger room and shared-participant warnings, cancel and verify the draft/error, then confirm and verify the resulting entry. |
| `/schedule` calendar, list and timeline | Select a day, change month and return to today. Toggle **自分の予定だけ表示** off/on/off. Seed owned, joined and unrelated items of both kinds. Verify list groups and timeline lanes, an empty filtered list, and today/own-item markers. Show room overlap and participant warnings in cards and timeline, including a conflicting item hidden by the filter. |
| `/schedule` details and edits | Open booking and external/work-from-home/leave details. Exercise owner edit, non-owner join/leave and cancellation. Open **recurring edit dialogs** for a booking and an external appointment, select each of single/following/all, capture selected controls and scope-specific help, then cancel or save. Compare affected dates and IDs. Exercise all three deletion confirmations on reset fixtures. |
| `/notifications` | Empty, unread/read and warning entries. Open a reservation notice and its detail, mark all read, delete one/all and check the badge. Exercise a schedule-only notice separately. |
| `/settings`, `/users` | Select both themes and check the shared layout. Capture the admin registration form, populated list, edited name/Chatwork fields and validation feedback. Check the non-admin redirect to home. |

## First Go slice — ReservationOverlapChecker.IsConflicting

`MeetingRoomBooker.Api/Services/Reservations/ReservationOverlapChecker.cs` compares two `ReservationModel` values. It returns a boolean without I/O or mutation. Room strings must match exactly, `left.Date.Date == right.Date.Date`, `left.StartTime < right.EndTime` and `left.EndTime > right.StartTime`. Compare full start/end timestamps and the separate calendar-date field.

Use the same left booking for all rows (room A, June 1, 2026, 10:00–11:00). Each right interval uses the date in its row.

| Case | Right booking | C# result | Coverage at the reference commit |
| --- | --- | --- | --- |
| Partial overlap | A, June 1, 10:30–11:30 | true | Direct overlap test uses 10:00–12:00 with the same start. The detection-service test covers this staggered interval. |
| Adjacent boundary | A, June 1, 11:00–12:00 | false | Direct adjacent-ranges test. |
| Different room | B, June 1, 10:00–11:00 | false | Direct different-rooms test. |
| Different day | A, June 2, 10:00–11:00 | false | No direct different-date test. Include this row in the shared fixtures. |

Direct tests live in `MeetingRoomBooker.Tests/ReservationOverlapCheckerTests.cs`. Production callers are the within-request series check in `ReservationsController.cs` and the pair scan in `RoomConflictDetectionService.cs`. Persisted create conflicts use a separate EF predicate in `MeetingRoomBooker.Api/Services/Reservations/ReservationConflictService.cs`. Preserve the caller's 409/override behavior and the detector's insert/deduplication separately from boolean parity.

**Verified in WSL.** All four rows above ran against the reference C# source in an isolated local runner. The existing checker and detection-service test classes passed (3 + 4 tests, including `MeetingRoomBooker.Tests/RoomConflictDetectionServiceTests.cs`). The first Go change should run the same rows through both languages and compare the booleans.

## Recovery to C# and SQLite

Rehearse each stage in development with synthetic rows and a recording fake Chatwork sender. `scripts/deploy/deploy-prod.sh` uses SQLite's backup API and integrity check. `scripts/deploy/rollback-prod.sh` restores the earlier API, web assets and final SQLite snapshot. A rollback after Go accepts writes also needs the committed PostgreSQL state.

| Stage | Data and action for returning to C# | Passing rehearsal |
| --- | --- | --- |
| Before cutover | Keep C#/SQLite authoritative. Retain matching API/Blazor builds, configuration, cookie-protection keys and a consistent SQLite backup with schema history. Prepare a reversible mapping for every table in `MeetingRoomBooker.Api/Data/AppDbContext.cs`. | Restore to an isolated C# instance. Run SQLite integrity checks, compare the reconciliation fields below, test login/remember-me and role-restricted actions, then perform sample booking and schedule edits. Record restore time. |
| While writes are stopped | Block incoming writes, drain requests/sends and stop both workers. Account for auth endpoints that can update users. Take and identify the final consistent SQLite snapshot. Import to PostgreSQL with delivery disabled and retain the untouched snapshot. | Compare both stores at the same cutoff. On any discrepancy, discard the candidate, restore the final snapshot and matching C# assets/config, verify reads and roles, then enable only the C# writer and workers. Prove no accepted write occurred after the cutoff. |
| After Go resumes writes | Freeze and drain Go, its workers and sends again. Snapshot PostgreSQL at the final committed transaction. Reverse-export its complete state into a fresh copy of the C#-compatible schema, preserving IDs, original delivery keys and deleted-row absence. Import rows directly with side effects disabled. | Under Go, create, change and delete bookings and work schedules (including series), change membership, read/delete notices and update a conflict record. After reversal compare every surviving row and deleted ID against PostgreSQL. Test C# login, next-ID allocation, list/detail and edits. Keep writes frozen on mismatch. Enable only C# after reconciliation. |

**Data reconciliation.** Compare counts and canonical row values by primary key across Users, Reservations, WorkScheduleEntries, Notifications, ChatworkDeliveryLogs and RoomConflictRecords. Check password-hash compatibility, admin flags, calendar dates/times, nulls, enum values, participant ID lists (comma-separated in SQLite), series IDs including legacy nulls, notification targets/read flags and conflict detection keys. Carry forward ID allocation high-water marks, including IDs of deleted rows.

**Notification rehearsal.** Preserve each delivery's recipient, target, type, key, status, attempt/sent times and scheduled start. The direct-send services consult existing keys for all statuses, and update keys contain a fresh change ID (`MeetingRoomBooker.Api/Services/Chatwork/ChatworkDeliveryKeys.cs`). Replaying write APIs would regenerate side effects. At a reminder boundary, run the fake sender through cutover, rollback and worker restart. Assert one receipt per expected event/recipient, zero resends for logged keys and the expected in-app rows. Simulate interruption between send and log commit. Reconcile ambiguous receipts before resuming delivery.

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

## Go reservation list

Blazor sends `GET /api/Reservations` with the session cookie. At `d8fe3b7`, the authorized controller reads all reservations without an ordering clause. For the WSL comparison, the reference app ran against a fresh SQLite database with the Chatwork and room conflict workers disabled. A synthetic member signed in through `/api/Auth/login`. The HTTP responses were `401` with an empty body and no redirect for anonymous access, `200` with `[]` for an empty table, and `200` with the four synthetic reservations captured in [the C# response fixture](../backend/internal/httpapi/testdata/reservation_list_csharp.json). Comparisons match reservations by ID and keep participant IDs in their original order.

The [first PostgreSQL migration](../backend/migrations/0001_create_reservations.sql) uses the same integer IDs and stores participant IDs as an ordered `integer[]`, avoiding the parsing and validation needed for SQLite's comma-separated text. A participant join table would allow foreign keys and per-participant queries. The array is sufficient for reading the list and keeps user tables out of this migration. The response derives `userName`, `roomName`, `participantCount`, and `isInternal` from the stored fields as the C# model does. `seriesId` and `repeatUntil` allow nulls, including recurring reservations with no legacy series ID.

The four date/time columns store .NET `DateTime` ticks, which are 100 ns intervals from `0001-01-01T00:00:00`. PostgreSQL `timestamp without time zone` would support native calendar operations and retain up to six fractional digits. The measured C# response includes `2026-06-10T08:00:00.1234567`. Tick values preserve that precision, retain chronological ordering, and require conversion for native calendar operations. The handler returns the stored wall-clock values with trailing fractional zeros trimmed and without a time-zone suffix.

In WSL, set `DATABASE_URL` to the local development database and apply the migration once from `backend`.

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_create_reservations.sql
```

Run `go test -v ./...` from `backend` for HTTP tests with an injected reader and identity. For the PostgreSQL comparison, set `RESERVATION_TEST_DATABASE_URL` to a disposable local database with permission to create a schema and run the command below. The test creates and drops its own schema, applies the migration to it, and inserts the [same synthetic rows](../backend/internal/httpapi/testdata/reservation_list_seed.sql) used for the SQLite capture. The Go handler runs on an HTTP test server for this comparison. The runnable API also serves the route through the session middleware.

```bash
RESERVATION_TEST_DATABASE_URL='postgres://meetingroombooker_dev:dev-password@127.0.0.1:5432/meetingroombooker_reservation_test?sslmode=disable' \
  go test -v -count=1 ./internal/httpapi -run '^TestReservationListPostgres$'
```

**Verified in WSL with PostgreSQL 16.15.** The `401` response had an empty body and no redirect. The empty and populated responses matched the C# `200` responses, including every JSON field on all four reservations.

## Go session authentication

The C# reference at `d8fe3b7` was run with a fresh SQLite database and synthetic member and administrator accounts. Blazor sends the session cookie on `POST /api/Auth/login`, `GET /api/Auth/me`, and `POST /api/Auth/logout`. The requests and results below were measured over HTTP, with a separate HTTPS login check and a process-local clock shift for cookie renewal.

| Request | C# result |
| --- | --- |
| Missing or invalid login fields, or malformed JSON | `400` with validation Problem Details. Missing or unsupported media type yielded `415`. |
| Incorrect password or unknown account | `401` with `Invalid email or password.` as text, or a JSON string with `Accept: application/json`. |
| Member or administrator login and `/me` | `200` with `id`, `name`, `email`, `avatarColor`, `isAdmin`, `chatworkAccountId`, and `chatworkDirectRoomId`. The member's Chatwork fields were null. |
| Anonymous `/me` or logout | `401` with an empty body and no redirect. |
| Authenticated logout | `204` with an empty body and a deletion cookie. The original C# cookie was still accepted when sent again. |

The Go handlers preserve these status and profile contracts. Malformed JSON uses Go validation messages and the Problem Details response omits ASP.NET's request trace identifier. Logout deletes the PostgreSQL session, so presenting the same Go cookie afterwards returns an empty `401`.

The C# application generated Identity V3 PBKDF2-HMAC-SHA512 hashes with 100,000 iterations, a 16-byte salt, and a 32-byte subkey. Go verifies that format using `crypto/pbkdf2` and a constant-time subkey comparison. The verifier also reads the Identity V2 and V3 SHA1/SHA256 formats accepted by ASP.NET Identity. It bounds encoded hashes at 256 bytes and iterations at 1,000,000. Requiring password resets during migration would avoid the compatibility parser and interrupt existing sign-ins. The [test vectors](../backend/internal/auth/testdata/identity_vectors.json) include hashes captured from both synthetic C# users and legacy vectors generated with .NET. Seed normalized email addresses and Identity password hashes when preparing users for Go. Plaintext password storage and password rehashing are outside this implementation.

The [PostgreSQL auth migration](../backend/migrations/0002_create_users_and_sessions.sql) stores profiles and active sessions. Each login issues a 32-byte cryptographically random token, with only its SHA-256 digest in PostgreSQL. The session lookup checks expiry and joins the user before any reservation read. A self-contained signed ticket would avoid the database lookup and require separate revocation state. The table gives logout immediate server-side revocation, and successful sign-ins remove expired rows.

C# measured `HttpOnly`, `SameSite=Lax`, `Path=/`, and `Secure` on HTTPS. The Go cookie is host-only with these attributes and uses `Secure` by default. Explicit `APP_ENV=development` allows loopback HTTP and is rejected on a non-loopback bind. Without “remember me” the cookie is a browser-session cookie. With it, the cookie expires after 14 days. Both modes have 14-day server-side expiry and renew once past the halfway point, as measured in C#. Go keeps the session ID on renewal so simultaneous requests can continue using it. The Go cookie is named `MeetingRoomBooker.GoSession`. Deciding whether to require a fresh login or migrate old `MeetingRoomBooker.Auth` sessions belongs to cutover planning.

Login and logout use Go's `CrossOriginProtection` to reject untrusted browser requests using `Sec-Fetch-Site` and `Origin`. `SameSite=Lax` also limits cross-site cookie sends. A per-request CSRF token would require a change to the Blazor client. For a separate frontend origin, `FRONTEND_ORIGINS` takes a comma-separated list of exact, trusted origins and enables credentialed CORS for them. HTTP origins require loopback development. Use the same loopback hostname for the API and frontend when testing cookies across local ports. For HTTPS deployments, preserve the external host at the reverse proxy and use the default secure-cookie setting.

In WSL, export `DATABASE_URL` for the local development database and apply the migrations once from `backend`. For a database with `0001` already applied, start with `0002`.

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_create_reservations.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0002_create_users_and_sessions.sql
APP_ENV=development go run ./cmd/api
```

For the smoke test, set `AUTH_TEST_DATABASE_URL` to a disposable PostgreSQL URL with permission to create schemas. The test applies both migrations in a temporary schema, seeds the synthetic users and reservations, runs the normal Go API startup on a loopback port, and removes the schema afterwards.

```bash
AUTH_TEST_DATABASE_URL='postgres://meetingroombooker_dev:dev-password@127.0.0.1:5432/meetingroombooker_auth_test?sslmode=disable' \
  go test -v -count=1 ./cmd/api -run '^TestSessionAuthenticationPostgres$'
```

**Verified in WSL with PostgreSQL 16.15.** The HTTP test passed for anonymous access before the reservations table existed, correct and incorrect credentials, both user profiles, reservation listing, session and persistent cookies, sliding renewal, logout, replay after logout, and expiry. The database-free tests cover input validation, the secure-cookie policy, and CSRF rejection on both login and logout.

## Go single reservation creation

Blazor sends `POST /api/Reservations` for a single reservation and appends `?allowOverlap=true` after confirmation. The C# reference at `d8fe3b7` ran with a disposable SQLite database and a recording Chatwork client. A successful request returned `201` with the reservation JSON and an absolute `Location` ending in `/api/Reservations?id=<id>`. Anonymous requests returned an empty `401`. Required fields, participant count, and an invalid overlap flag produced `400` Problem Details, while an end time at or before the start returned a Japanese string with `400`. The overlap response was `409` with the room, date, time, and existing reservation label. It was plain text by default and a JSON string for `Accept: application/json`. Adjacent times and explicitly allowed overlaps both returned `201`. The [C# HTTP fixtures](../backend/internal/httpapi/testdata/reservation_create_csharp.json) contain the measured responses used by the Go tests.

Go reuses session authentication and origin protection, takes organizer details from the authenticated profile, and returns the same JSON projection as the reservation list. Request aliases share their canonical fields in JSON order. Nonpositive and duplicate participant IDs are removed. The organizer is prepended when absent, and an existing organizer entry keeps its position. The start and end retain their times of day and use the selected calendar date. The original `date` value is preserved even when it includes a time of day. Date-time values use the list's zone-free format. C# could include a time-zone suffix in the immediate POST response for zoned input and returned zone-free values on its subsequent GET. Go uses the latter serialization for both. Go returns stable Problem Details messages for malformed JSON and omits ASP.NET trace IDs. This route still returns `400` for recurring input. The Go series endpoint accepts a list of occurrences.

The PostgreSQL transaction takes an advisory lock for the room and calendar day, then performs the overlap read in a separate Read Committed statement so it sees the prior writer's commit. It uses strict start-before-end and end-after-start comparisons, including rows whose stored date has a time of day. The transaction inserts while holding the lock. Override requests take the same lock. The conflict query still matches the full room string, so a room hash collision only broadens lock contention and index candidates. A table-wide lock would serialize unrelated rooms and dates. A plain exclusion constraint would also reject the intentional overlaps. Future reservation write paths need to participate in the same locking protocol.

Apply [migration `0003`](../backend/migrations/0003_enable_reservation_creation.sql) after `0001` and `0002` and after loading any existing reservation IDs. It adds an identity sequence initialized from the existing maximum and an index for the conflict lookup. The sequence supplies IDs even when the request contains an `id`. A later explicit ID import requires resynchronizing the sequence with writers stopped and accounting for the legacy ID high-water mark.

```bash
# WSL, from backend with DATABASE_URL set to the local database
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0003_enable_reservation_creation.sql
```

C# persists the booking, writes in-app notices for participants other than the organizer, and invokes direct Chatwork delivery for configured stakeholders. The first synthetic C# booking produced one in-app notice and two successful fake Chatwork deliveries with delivery log rows. See [Go notifications on single reservation creation](#go-notifications-on-single-reservation-creation) for in-app generation and its database requirements. Chatwork delivery is a later migration step.

For verification, set `RESERVATION_CREATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL that permits creating schemas. The test uses the normal Go server, migrates a schema with existing IDs, and compares HTTP responses with the C# fixture. It holds the room/date lock until two requests are waiting to check that one returns `201`, one returns `409`, and the database contains one new reservation and one notification.

```bash
# WSL, from backend
go test -v -count=1 ./cmd/api -run '^TestSingleReservationCreatePostgres$'
```

## Go work schedule list

Blazor sends `GET /api/work-schedule-entries` with the session cookie and optional `from` and `to` dates formatted as `yyyy-MM-dd`. At `d8fe3b7`, this route returns all matching entries to an authenticated user. It orders by date, start time or date when the start is null, numeric type, and ID. Both range bounds are inclusive and are taken at midnight. Entries with a non-midnight legacy date on the last day are therefore outside the range. Missing or empty bounds default independently to seven days before and 31 days after the server's local date. With explicit offsets, ASP.NET normalizes to UTC before taking the date.

The local C# HTTP capture returned an empty `401` for anonymous access, a `400` with `取得開始日は取得終了日以前にしてください。` for an inverted explicit range, and `400` Problem Details for a malformed date. An inferred inverted range returned `200` with `[]`. The [capture fixture](../backend/internal/httpapi/testdata/work_schedule_list_csharp.json) preserves all fields of 11 synthetic entries and the order of each response. The Go test compares numeric types and leave periods, nullable times and recurrence fields, participant order, empty arrays, and seven fractional second digits. Go omits the ASP.NET trace identifier and supports the Blazor date format, the measured ISO date-time forms, and common numeric date forms.

The [PostgreSQL migration](../backend/migrations/0004_create_work_schedule_entries.sql) keeps explicit IDs, ordered integer participant arrays, and .NET ticks with the same conversion used for reservations. Native timestamps would allow PostgreSQL calendar arithmetic and store up to six fractional digits. Tick storage preserves the measured precision and the legacy time of day. The query uses the date index for the bounded lookup and expresses the C# ordering explicitly.

```bash
# WSL, from backend with DATABASE_URL set to the local development database
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0004_create_work_schedule_entries.sql
```

For the PostgreSQL comparison, set `WORK_SCHEDULE_TEST_DATABASE_URL` to a disposable local database that permits creating schemas. The test starts the Go server and compares the HTTP output with the C# capture. It shifts the fixture dates together for repeatable relative-window checks on later days.

```bash
# WSL, from backend
go test -v -count=1 ./cmd/api -run '^TestWorkScheduleListPostgres$'
```

For the browser check, apply migrations `0001` through `0004` and seed a synthetic member, a 10:00–10:30 booking, and an external appointment, a work-from-home entry, and a leave entry on the browser's selected day. Point a temporary copy of Blazor's `ApiBaseUrl` to the Go API and add the exact Blazor origin to `FRONTEND_ORIGINS`. In Chromium, open the reservation form and verify its preview. Submit a 09:00–10:00 reservation in the same room and verify the success dialog and the calendar. The WSL check observed `200` for the work schedule GET and `201` for the POST. Participant and notification endpoints returned `404` during the same run.

## Go participant list

Blazor sends `GET /api/Users/participants` with the session cookie and omits the signed-in user from the reservation picker. At `d8fe3b7`, the C# endpoint returned `200` with identical lists for a member and an administrator. Each row contained `id`, `name`, and `avatarColor`, in ascending ID order. The anonymous response was an empty `401`. The [C# HTTP fixture](../backend/internal/httpapi/testdata/participant_list_csharp.json) records the three synthetic users returned to both roles.

Go reuses the session middleware and selects the three columns directly from the existing `users` table in ID order. A dedicated response type keeps the endpoint's projection independent of the full user profile and admin operations. The PostgreSQL test seeds users out of order and compares the exact JSON for both roles. It also checks the anonymous response before the user table exists.

```bash
# WSL, from backend with PARTICIPANT_TEST_DATABASE_URL set to a disposable database
go test -v -count=1 ./cmd/api -run '^TestParticipantListPostgres$'
```

For the browser check, apply migrations `0001` through `0004` and seed a synthetic member and a second participant. Point a temporary Blazor copy at the Go API and set `FRONTEND_ORIGINS` to the Blazor origin. Sign in as the member, open the reservation form, select the second user, and submit a booking in an available time slot. Chromium observed `GET 200` for the picker and `POST 201` for the booking. The request sent participant ID `17`, PostgreSQL stored `[2, 17]` including the organizer, and the calendar displayed the selected participant's name.

## Go notification inbox

Blazor calls `GET /api/Notifications/user/{userId}`, `PUT /api/Notifications/{id}/read`, and `DELETE /api/Notifications/{id}`. At `d8fe3b7`, the recipient and administrators could list and change the recipient's notifications. Other members received `403` for existing notifications, and anonymous requests received `401`. The list returned newest first or `[]` when empty. Successful mutations returned `204`, including repeated requests to mark a notification as read. Missing numeric IDs returned `404` Problem Details, and invalid ID paths returned an empty `404`. The [C# HTTP fixture](../backend/internal/httpapi/testdata/notification_inbox_csharp.json) captures all 24 cases with request-specific trace IDs removed.

Go reads the user ID and administrator flag from the session. Mutations check the owner while holding the notification row lock and finish in the same transaction. An authorization lookup followed by a separate write could race with deletion. A write filtered only by owner would also merge the `403` and `404` outcomes. `PUT` and `DELETE` use the existing origin protection. Credentialed CORS preflights advertise these methods for configured frontend origins.

The [PostgreSQL migration](../backend/migrations/0005_create_notifications.sql) preserves explicit IDs, nullable reservation and work schedule targets, and .NET tick precision. The target IDs retain the values from the C# model, including notifications that refer to records that are absent. The index supports lookup by recipient and creation time.

```bash
# WSL, from backend with DATABASE_URL set to the local development database
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0005_create_notifications.sql
```

Set `NOTIFICATION_TEST_DATABASE_URL` to a disposable local PostgreSQL URL that permits creating schemas. The test compares statuses, content types, and JSON against the C# fixture and covers both trusted preflights and denied cross-origin writes using a valid session.

```bash
# WSL, from backend
go test -v -count=1 ./cmd/api -run '^TestNotificationInboxPostgres$'
```

The browser check used a temporary Blazor copy and two seeded notices for a synthetic member, one for a reservation and one for a work schedule. Chromium displayed the notifications in order. `MARK ALL READ` sent two successful PUTs, and a browser reload showed both rows as read. Deleting one notification returned `204`, and a second reload showed only the remaining read notification. The browser recorded `204` preflights for PUT and DELETE.

## Go administrator notification creation

Blazor sends the notification model to `POST /api/Notifications`. A WSL run of C# at `d8fe3b7` with synthetic users and a disposable SQLite database returned empty `401` and `403` responses for anonymous users and members. The member received `403` even for malformed JSON. Administrators received `400` Problem Details for malformed JSON, invalid numbers and null non-nullable fields, and `415` for a non-JSON content type. Successful requests returned `201`, the full notification JSON, and an absolute `Location` ending in `/api/Notifications/user/{userId}`. The [C# capture](../backend/internal/httpapi/testdata/notification_create_csharp.json) includes 18 requests and the matching SQLite rows for all eight successful requests.

Omitted values defaulted to user ID zero, an empty message, `Info`, an unread flag, and local server time for both dates. The two target IDs accepted both omission and explicit `null`. C# also accepted arbitrary types and IDs pointing to absent records. Input without a zone kept its wall time, UTC input kept `Z` in the POST response, and numeric offsets were converted to the server zone. SQLite stored the wall time with up to seven fractional digits. Go preserves these values in ticks and reuses the inbox JSON fields. Go uses stable Problem Details messages for malformed JSON and type conversion errors. ASP.NET's trace ID, field path and byte position in parser diagnostics are framework-specific.

[Migration `0006`](../backend/migrations/0006_enable_notification_creation.sql) adds a PostgreSQL identity sequence to the existing notification ID column and initializes it from the highest positive ID. The primary key and existing rows keep their IDs. A sequence allocates distinct generated IDs across concurrent inserts. Computing `MAX(id) + 1` per request would require locking all creates and could reuse a deleted ID. C# also accepted a nonzero caller-supplied ID. Go preserves it and advances the sequence when a positive supplied ID reaches its current value. These exceptional inserts take a table lock that allows ordinary reads and coordinates with other writes. Generated-ID requests use a single INSERT. A duplicate explicit ID surfaces as `500`, with an empty body in Go. If a later legacy import uses explicit IDs directly in SQL, stop writers and reconcile the sequence with the imported IDs and the source high-water mark before resuming.

```bash
# WSL, from backend after migration 0005 and any initial legacy ID import
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0006_enable_notification_creation.sql
# Set NOTIFICATION_CREATE_TEST_DATABASE_URL to a disposable PostgreSQL URL with schema creation privileges.
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestAdminNotificationCreatePostgres$'
```

The PostgreSQL 16 integration test matched the C# statuses, successful JSON, Location paths and stored fields. With existing ID `4100`, it issued `4101`. After explicit ID `4800`, it issued `4801`, then issued `4802` even after deleting `4801` and adding a lower ID. A valid admin cookie from an untrusted origin received `403`. The recipient listed the new notification, marked it read, verified the saved flag, deleted it, and confirmed its absence on the next GET. Go unit tests, static analysis, and the Git whitespace check passed in WSL.

## Go notifications on single reservation creation

After a single reservation is created, each normalized participant other than the organizer receives one unread in-app notification linked to the reservation ID and date. The C# controller at `d8fe3b7` produced `Info` for a booking without participant conflicts, `Warning` when a timed external appointment strictly overlapped the booking, and no rows for a booking with only the organizer. The [SQLite capture](../backend/internal/httpapi/testdata/reservation_create_notifications_csharp.json) records the requests and stored notices. Go includes the organizer in the overlap check and gives each recipient the same warning text, listing up to five conflicts and the remaining count. When conflict start times match, participant names are ordered using the database collation.

Apply migrations `0004` through `0006` before accepting single-reservation writes with participants. Go uses the existing work schedule and notification tables and the notification identity sequence. For an existing matching notice, it refreshes the newest row to unread and removes older matches, as in the C# service. C# saved the reservation and notices separately. Go writes both in the existing room/date transaction, so a notice write failure rolls back the reservation along with the notice rows.

Set `RESERVATION_CREATE_TEST_DATABASE_URL` to a disposable PostgreSQL URL that permits creating schemas and run the tests from `backend`.

```bash
go test -v -count=1 ./cmd/api -run '^Test(SingleReservationCreate|ReservationCreateNotifications)Postgres$'
```

The PostgreSQL 16.2 checks exercised the real Go HTTP endpoints. The recipient inbox matched SQLite for the notice text, type, read flag and target date, including seven fractional digits. The tests also checked the generated reservation link, duplicate refresh, a rejected room overlap, concurrent creation, and a forced notification-write failure.

## Go single reservation update

Blazor sends `PUT /api/Reservations/{id}?notifyParticipants=true|false`. The Go route checks the body ID against the route ID and grants update access to the organizer and administrators. It preserves the stored organizer ID, name, and series ID. An update can save an overlapping booking as it does in C#. The row lock serializes edits to the same reservation, and the destination room/date lock coordinates the edit with concurrent creates.

Notifications default to enabled. Added participants get an invitation, removed participants get an `Info` removal notice, and retained participants get a summary when tracked fields change or a timed external appointment overlaps. Go reuses the single-create notice formatting and upsert. The booking and its notices commit together. The C# reference saved them in separate operations.

Set `RESERVATION_UPDATE_TEST_DATABASE_URL` to a disposable PostgreSQL URL that permits creating schemas and run the following from `backend`.

```bash
go test -v -count=1 ./cmd/api -run '^TestSingleReservationUpdatePostgres$'
```

The [C# `d8fe3b7` capture](../backend/internal/httpapi/testdata/reservation_update_csharp.json) records the rows from four controller calls with SQLite: participant changes, a warning-only edit, an admin edit with notices disabled, and an edit without changes or warnings. The Go HTTP test checks the resulting reservation and notices on PostgreSQL 16.2. It also verifies authentication, authorization, input errors and rollback after a forced notice write failure.

## Go single reservation cancellation

Blazor sends `DELETE /api/Reservations/{id}`. The organizer and administrators can cancel the addressed booking. The [C# SQLite capture](../backend/internal/httpapi/testdata/reservation_cancel_csharp.json) records successful, forbidden, missing, and repeated cancellations. The single-row delete preserves sibling rows with the same series ID and existing in-app notices. C# sends Chatwork cancellation notices after deletion. Delivery parity is covered by the Chatwork migration.

Go uses the same row lock as updates for the authorization check and the same room/date lock as creates. A create that waits behind a cancellation checks the freed slot after the delete commits. The integration test also checks anonymous and untrusted-origin requests, concurrent cancellations of one row, and reuse of the freed slot.

Set `RESERVATION_CANCEL_TEST_DATABASE_URL` to a disposable PostgreSQL URL that permits creating schemas and run the following from `backend`.

```bash
go test -v -count=1 ./cmd/api -run '^TestSingleReservationCancellationPostgres$'
```

## Go reservation participation

Blazor sends `POST /api/Reservations/{id}/join` and `POST /api/Reservations/{id}/leave`. An authenticated user changes their own membership. A repeated request already in the desired state returns `204` without changing data. The organizer's leave returns `400` with the C# message. Changes apply to the addressed booking, including when it belongs to a recurring series.

Go locks the reservation row, normalizes the participant IDs and legacy reservation fields, and saves the membership and organizer's notice in one transaction. It preserves the booking's headcount and display text as C# does. A successful change by another user adds or refreshes an unread `Info` notice using the C# message prefix. Duplicate prefix matches are removed while retaining the newest notice ID.

The [C# SQLite capture](../backend/internal/httpapi/testdata/reservation_participation_csharp.json) records eight cases. The PostgreSQL test compares the booking list and organizer's inbox through the Go API, and checks that concurrent joins keep both members and a forced notice write failure rolls back the membership change.

Set `RESERVATION_PARTICIPATION_TEST_DATABASE_URL` to a disposable PostgreSQL URL that permits creating schemas and run from `backend`.

```bash
go test -v -count=1 ./cmd/api -run '^TestReservationParticipationPostgres$'
```

## Go recurring reservation creation

Blazor's `AddReservationsAsync` submits a pre-expanded array to `POST /api/Reservations/series` and adds `?allowOverlap=true` after overlap confirmation. The API stably sorts by the supplied date and start date-time before normalizing each row. It takes the organizer from the authenticated session. Each array element becomes a reservation with its own database-generated `id`. For example, three weekday occurrences save three reservation rows with one new shared `seriesId`. An entry with repeat type `しない` or no repeat end date has a null `seriesId`, even in an array containing recurring entries. A successful response returns `201`, the ordered array, and an absolute `Location` ending in `/api/Reservations`.

Empty and null arrays return `400`. A recurring occurrence on Saturday or Sunday, an end time at or before its start, or a repeat end day before the occurrence day also returns `400`. The repeat end day is inclusive. Partial overlaps within the array or against stored reservations return `409` with the conflicting date, room, time, and label. Adjacent bookings are accepted. `allowOverlap=true` bypasses both overlap checks and still applies the validation rules.

The first saved occurrence represents the request for notifications. The union of participants across all rows receives one unread notice per person other than the organizer. Each notice links to the first occurrence's ID and date. If the representative is recurring, its message counts rows sharing its `seriesId`. If it is standalone, the notice uses the single-reservation message. The external-appointment warning checks the representative's slot against the participant union, including the organizer. It uses the existing database collation for equal-time warning lines.

Go takes the same transaction-level advisory locks as single booking creation, in ascending order of the actual PostgreSQL room hash and calendar day. Sorting the lock keys separately from the response order also handles room hash collisions. Read Committed conflict reads run after the locks are acquired. A table lock would block unrelated rooms and dates. An exclusion constraint would also block intentional overlaps. Reservations and in-app notices commit together. The C# reference saved them separately and kept two bookings when an SQLite trigger forced the notification insert to fail. Go returned `500` with zero new rows in both tables when a notice failed or the second reservation insert failed. Existing migrations `0001` through `0006` supply the schema.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/reservation_series_create_csharp.json) records 19 synthetic requests. To reproduce it, run a temporary checkout of `d8fe3b7` on WSL with a fresh SQLite file, `Database__EnsureCreatedOnStartup=true`, `Database__ApplyMigrationsOnStartup=false`, and a fake Chatwork sender. Set `Chatwork__Enabled=false` and `RoomConflictDetection__Enabled=false`. Create a synthetic administrator (ID 1), the organizer `架空の予約者` (2), and the participants `架空の参加者` (3) and `架空の別参加者` (4). Before each case, clear reservations, notifications and work schedule entries, then seed its `existing` reservations and `appointments`. Send each captured array and query string with `Accept: application/json` and the organizer's cookie for authenticated cases. Compare the response and SQLite rows. Match generated booking IDs and series IDs by row and verify that notification targets resolve to the first saved row. An SQLite `BEFORE INSERT` trigger on `Notifications` that raises an error reproduces the separate-save result.

For the Go comparison, set `RESERVATION_SERIES_CREATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges and run from `backend` in WSL. The integration test applies migrations, seeds the same cases, and uses the normal HTTP server. Without the database URL, the test is skipped.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestRecurringReservationCreatePostgres$'
```

The PostgreSQL 16.15 run matched the captured statuses, ordered response fields, Location, stored rows, and notices. C# returned a `Z` suffix in the POST response for UTC input. Go uses the existing zone-free reservation JSON, which matches the stored wall time. Go also allocates its own booking IDs and uses its shared JSON error format. Concurrent series and single requests waited on the same lock and returned one `201` and one `409`. Two series with opposite room order also acquired the same first lock. Series updates, deletions, and Chatwork delivery are separate migration steps. Blazor sends a single candidate through the single-create endpoint, so a one-occurrence recurrence submitted through that UI path still receives its existing Go `400`.

## Recovery to C# and SQLite

Rehearse each stage in development with synthetic rows and a recording fake Chatwork sender. `scripts/deploy/deploy-prod.sh` uses SQLite's backup API and integrity check. `scripts/deploy/rollback-prod.sh` restores the earlier API, web assets and final SQLite snapshot. A rollback after Go accepts writes also needs the committed PostgreSQL state.

| Stage | Data and action for returning to C# | Passing rehearsal |
| --- | --- | --- |
| Before cutover | Keep C#/SQLite authoritative. Retain matching API/Blazor builds, configuration, cookie-protection keys and a consistent SQLite backup with schema history. Prepare a reversible mapping for every table in `MeetingRoomBooker.Api/Data/AppDbContext.cs`. | Restore to an isolated C# instance. Run SQLite integrity checks, compare the reconciliation fields below, test login/remember-me and role-restricted actions, then perform sample booking and schedule edits. Record restore time. |
| While writes are stopped | Block incoming writes, drain requests/sends and stop both workers. Account for auth endpoints that can update users. Take and identify the final consistent SQLite snapshot. Import to PostgreSQL with delivery disabled and retain the untouched snapshot. | Compare both stores at the same cutoff. On any discrepancy, discard the candidate, restore the final snapshot and matching C# assets/config, verify reads and roles, then enable only the C# writer and workers. Prove no accepted write occurred after the cutoff. |
| After Go resumes writes | Freeze and drain Go, its workers and sends again. Snapshot PostgreSQL at the final committed transaction. Reverse-export its complete state into a fresh copy of the C#-compatible schema, preserving IDs, original delivery keys and deleted-row absence. Import rows directly with side effects disabled. | Under Go, create, change and delete bookings and work schedules (including series), change membership, read/delete notices and update a conflict record. After reversal compare every surviving row and deleted ID against PostgreSQL. Test C# login, next-ID allocation, list/detail and edits. Keep writes frozen on mismatch. Enable only C# after reconciliation. |

**Data reconciliation.** Compare counts and canonical row values by primary key across Users, Reservations, WorkScheduleEntries, Notifications, ChatworkDeliveryLogs and RoomConflictRecords. Check password-hash compatibility, admin flags, calendar dates/times, nulls, enum values, participant ID lists (comma-separated in SQLite), series IDs including legacy nulls, notification targets/read flags and conflict detection keys. Carry forward ID allocation high-water marks, including IDs of deleted rows.

**Notification rehearsal.** Preserve each delivery's recipient, target, type, key, status, attempt/sent times and scheduled start. The direct-send services consult existing keys for all statuses, and update keys contain a fresh change ID (`MeetingRoomBooker.Api/Services/Chatwork/ChatworkDeliveryKeys.cs`). Replaying write APIs would regenerate side effects. At a reminder boundary, run the fake sender through cutover, rollback and worker restart. Assert one receipt per expected event/recipient, zero resends for logged keys and the expected in-app rows. Simulate interruption between send and log commit. Reconcile ambiguous receipts before resuming delivery.

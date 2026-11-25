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

C# persists the booking, writes in-app notices for participants other than the organizer, and invokes direct Chatwork delivery for configured stakeholders. The first synthetic C# booking produced one in-app notice and two successful fake Chatwork deliveries with delivery log rows. See [Go notifications on single reservation creation](#go-notifications-on-single-reservation-creation) for in-app generation and [Go Chatwork creation DMs](#go-chatwork-creation-dms) for post-commit delivery.

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

## Go user registration

`POST /api/Users/register` uses the existing write-origin checks and optional session authentication. The first successful registration on an empty database creates an administrator and can be anonymous. After that, only a stored administrator can create users, and each created user is a general user. The role and ID come from the database and server policy. A successful registration returns `201`, an absolute `Location` ending in `/api/Users?id={id}`, and the same public profile fields as login. The collection path matches the C# Location.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/user_registration_csharp.json) covers 27 cases. Body and model validation run before the role check. For a well-formed request in a populated database, C# returned an empty `403` for anonymous users and `201` for a general user. The C# expression coalesces the nullable authentication result with a branch beginning with true, so it never evaluates the role check. Go returns an empty `403` for both roles. Invalid models still return `400` first, and unsupported content types return `415`. The malformed JSON response uses Go's generic validation error and omits ASP.NET parser positions and trace IDs.

Required name, email and password reject blank values. Name and email are checked against 100 and 256 UTF-16 code units before trimming. Both optional Chatwork IDs are checked against 100 UTF-16 units before trimming and become null when blank. Email is trimmed and lowercased with invariant casing, which preserves capital dotted `İ` in the measured C# case. Registration and login share this normalization. Avatar color defaults to `#58a6ff` when omitted or blank. A nonblank color keeps its original text and explicit null fails model validation. The password keeps its original characters, including surrounding whitespace. Go uses the existing 64 KiB authentication request limit. In the oversized request case C# returned `201` and allowed login, and Go returned `413`.

Go hashes the password with the ASP.NET Identity V3 parameters measured in the reference, PBKDF2-HMAC-SHA512 with 100,000 iterations, a fresh 16-byte random salt and a 32-byte subkey. The tests verified distinct salts, Go login after registration, and verification of a Go-generated hash by ASP.NET's `PasswordHasher`. A password with its surrounding spaces removed was rejected. The capture records how to generate test input and contains no passwords or hashes.

A PostgreSQL `SHARE ROW EXCLUSIVE` lock on `users` guards the empty-table check, the stored actor role, the normalized email check and the insert in one read-committed transaction. This covers the empty-table case that has no row to lock and also coordinates with other writers to the table. Concurrent readers can proceed. The email unique constraint remains the database invariant. Two simultaneous anonymous requests produced one `201` and one `403` in Go. C# produced two `201` responses and two administrators in the observed run. For concurrent administrator requests with the same normalized email, Go returned `201` and `409`, and C# returned `201` and `500`. A normal duplicate returns `409` with `Email already exists.` as text or a JSON string, depending on Accept.

[Migration `0011`](../backend/migrations/0011_enable_user_registration.sql) adds a `GENERATED BY DEFAULT` identity and initializes the sequence from existing IDs. It preserves explicit IDs for imports. In PostgreSQL, an existing ID of `4200` remained and the first new user received `4201`. Apply the migration after loading the existing IDs, or advance the sequence before accepting writes after a later explicit-ID import.

To reproduce the C# capture in WSL, run a disposable `d8fe3b7` SQLite instance with Chatwork disabled as described in [Go session authentication](#go-session-authentication). Start with an empty users table. Submit the fixture cases in order with the recorded Accept and Content-Type. Generate a secret in memory for `normal`, surround it with spaces for `padded`, and repeat it until the request exceeds 64 KiB for `oversize`. Follow the indicated logins to obtain administrator and member sessions. After the sequential cases, submit two administrator requests for one email with different casing and padding. Reset the disposable users table and submit two simultaneous anonymous requests with different emails. Compare status codes and the resulting administrator count.

In WSL, set `USER_REGISTRATION_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. From `backend` run:

```bash
go test -v -count=1 ./cmd/api -run '^TestUserRegistrationPostgres$'
go test ./...
go vet ./...
```

The registration suite starts the normal API on loopback and uses isolated PostgreSQL schemas. It compares response and stored profile fields, validates login and origin protection, queues both concurrency cases at the database lock, and applies the identity migration over existing rows. All 26 PostgreSQL integration suites passed on PostgreSQL 16.15. The database-free `go test ./...` passed with those suites skipped. `go vet ./...`, formatting, and `git diff --check` passed.

## Go administrator user management

Blazor uses `GET /api/Users`, `PUT /api/Users/{id}/name`, `PUT /api/Users/{id}/chatwork-account`, `PUT /api/Users/{id}/chatwork-direct-room`, and `DELETE /api/Users/{id}`. Both APIs return `401` for anonymous access and `403` for authenticated general users. Go uses the current session role and requires a trusted origin for cross-origin writes. The list returns profiles in ID order with `id`, `name`, `email`, `avatarColor`, `isAdmin`, `chatworkAccountId`, and `chatworkDirectRoomId`. Following the actual registration Location `/api/Users?id=2` as an administrator returns the same collection.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/admin_users_csharp.json) records 68 cases from `d8fe3b7`. A nonnumeric ID returns an empty `404` before authentication. For a numeric ID, the role check comes first, JSON binding and model validation come next, and a missing user produces `404` Problem Details before value validation. Explicit `name: null` yields a model error even for a missing user. An omitted name becomes empty and produces `Name is required.` only after the user is found. Name and Chatwork IDs are trimmed before the 100 UTF-16 code-unit check. Emoji input at exactly 100 units succeeded, and 101 units returned `400` in both APIs. Empty, omitted, and null Chatwork fields clear only the selected ID. Successful writes return an empty `204`. Go uses its generic JSON validation message for parser and type failures and omits the ASP.NET trace ID.

Rename updates the stored display name on every reservation owned by that user. Delete checks for a missing user, then rejects deleting an administrator with `管理者ユーザーは削除できません。`, including when the administrator targets themself. The following self-delete guard uses `自分自身は削除できません。`. C# reaches that guard when a live administrator cookie outlasts a database demotion. Go reads the live role and returned an empty `403` for the demoted caller.

Deletion removes owned reservations, notifications that point to those reservations, and notifications addressed to the deleted user. On other reservations that included the user, it removes the ID, deduplicates remaining IDs in first-occurrence order, and recalculates the count, including zero. For instance, `[2,3,2,4,3,0,-1,4]` becomes `[3,4,0,-1]` when deleting user 2. Both databases retain the stored participant display text, work schedules, and historical Chatwork delivery logs. The C# conflict record also survived and has no equivalent table in the current Go schema. Both captures observed zero new notices and zero requests to the fake Chatwork sender.

Administrative writes lock the target user in a PostgreSQL read-committed transaction. Rename and delete lock affected reservations in ID order, and delete uses the existing room and date locks for owned reservations. Separate write statements without a transaction could expose a partially updated profile or incomplete cleanup. PostgreSQL rolls back the profile, reservations, notices, and sessions together on failure. The existing session foreign key revokes sessions on delete. In the comparison, a deleted user's old C# cookie still accessed the participant list until `/api/Auth/me` returned `401` with Problem Details. Go returned an empty `401` immediately on both routes.

To repeat the C# capture in WSL, use a temporary checkout of `d8fe3b7`. Set `ConnectionStrings__DefaultConnection` to a fresh SQLite file, `Database__EnsureCreatedOnStartup=true`, and `Database__ApplyMigrationsOnStartup=false`. In the temporary copy of `Program.cs`, point the Chatwork HTTP client at a recording loopback fake and remove the reminder worker registration. Start the API on loopback in Development with `Chatwork__Enabled=true`, a synthetic Chatwork token, and `RoomConflictDetection__Enabled=false`.

Register synthetic users `admin`, `target`, `member`, and `other-admin` in that order, using the corresponding `@example.test` emails, names prefixed with `Synthetic `, account IDs 11–14, and room IDs 21–24. Promote user 4 in SQLite before logging in. Use `seedAdminUserDependencies` in the Go integration test to seed corresponding SQLite rows, including reservation name and count aliases. Add a synthetic conflict record with ID 50, reservations 10 and 20, and reporting user 2. Replay the fixture in order using the captured Accept headers and JSON bodies. Follow the Location from user 2's registration. For `delete_demoted_self`, demote user 4 after login and restore the role after the request. For `deleted_participants`, reuse user 2's cookie before calling `/api/Auth/me`. [Go integration test](../backend/cmd/api/admin_users_postgres_test.go)

In WSL, set `ADMIN_USERS_TEST_DATABASE_URL` to a disposable PostgreSQL URL with schema creation privileges. The tests seed isolated schemas, start the normal Go API on loopback, and compare HTTP responses and stored rows. The mutation suite injects a constraint failure on a reservation rename and a trigger failure after delete cleanup, then checks rollback and continued session validity. It also forces each order of simultaneous rename and delete through PostgreSQL locks. Rename followed by delete returned `204` and `204`. Delete followed by rename returned `204` and `404`. Run from `backend`:

```bash
go test -v -count=1 ./cmd/api -run '^TestAdmin(Users|UserMutation)Postgres$'
go test ./...
go vet ./...
```

All 28 PostgreSQL integration tests ran and passed on PostgreSQL 16.15, including both administrator management tests. The database-free suite passed with the PostgreSQL tests skipped. Formatting, `go vet ./...`, and `git diff --check` passed.

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

Blazor sends `DELETE /api/Reservations/{id}`. The organizer and administrators can cancel the addressed booking. The [C# SQLite capture](../backend/internal/httpapi/testdata/reservation_cancel_csharp.json) records successful, forbidden, missing, and repeated cancellations. The single-row delete preserves sibling rows with the same series ID and existing in-app notices. See [Chatwork cancellation DMs](#go-chatwork-cancellation-dms) for post-commit delivery.

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

## Go recurring reservation update

Blazor sends `POST /api/Reservations/{id}/series-update` with `originalReservation`, `updatedReservation`, `scope`, and `notifyParticipants`. Both snapshots are model-validated. The stored row for the route ID determines authorization and series membership. The owner and administrators can edit, a different member receives an empty `403`, and unauthenticated requests receive an empty `401`. A missing numeric ID returns `404` Problem Details.

`scope` defaults to `single`. Unknown strings and scopes on a non-recurring base use the existing single update, which requires the nested updated ID to match the route and accepts the submitted recurrence settings. `following` and `all` on a recurring base retain each row's original ID, organizer, name, repeat type, repeat end, and series ID. The server applies the calendar-day difference between the stored base and the submitted date to every target's date, then uses the submitted start and end times of day. It validates the template and each resulting row. Both paths accept overlapping edits and return an empty `204` on success. A recurring edit may move dates onto a weekend.

For a nonblank series ID, the lookup compares the base's trimmed key with the stored values and restricts the result to the same organizer. A padded stored key can therefore exclude the base itself. For legacy rows, it matches organizer ID, name, room, type, purpose, repeat type, repeat end calendar day, and the start and end times of day. A row with its own series ID can still satisfy this legacy lookup. `following` adds an inclusive lower bound on the base calendar day, including earlier times on that day. An empty selection returns `204`.

The body flag `notifyParticipants` defaults to true. Go reuses the single-update notices for every changed occurrence. In the measured `following` case, three rows moved and each produced an added-participant notice and a removed-participant notice, linked to the edited row. PostgreSQL writes all reservations and their notices in one transaction. When a notice insert was forced to fail, C# returned `500` with four edited reservations already saved. Go returned `500` with all original reservations restored and no notices persisted. A failure on a later reservation update also left all rows unchanged. Go uses its shared JSON parser diagnostics for malformed requests.

The transaction takes all relevant row locks in ascending reservation ID order, including the base, before taking any room/day locks. It recomputes membership from the locked base in Read Committed. If the selection grew while waiting and now needs an unlocked row, it rolls back and reacquires the full set in ID order. Locking the base first could deadlock two overlapping edits anchored on different rows. For a batch update, the existing create lock keys are acquired in hash and day order for both origin and destination. This keeps unrelated slots available and lets a waiting create see the post-edit state. PostgreSQL tests observed `409` when a create waited at the moved-to slot, `201` when it waited at the vacated slot, and `201` followed by an overlapping `204` edit when the create took the destination lock first. No schema change was needed.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/reservation_series_update_csharp.json) contains 25 cases. To reproduce it, start a temporary `d8fe3b7` checkout with a new SQLite file, `Database__EnsureCreatedOnStartup=true`, `Database__ApplyMigrationsOnStartup=false`, and a fake Chatwork sender. Set `Chatwork__Enabled=false` and `RoomConflictDetection__Enabled=false`. Register synthetic users in this order: administrator `架空の管理者`, owner `架空の予約者`, other member `架空の参加者`, and added participant `架空の追加参加者`. Before each request, clear reservations and notices and seed the case's dataset. Send its body with `Accept: application/json` and the indicated role's cookie. Compare changed and unchanged reservations by ID and compare notices by target reservation and recipient. For the failure cases, install a SQLite `BEFORE UPDATE` trigger on reservation 12 or a `BEFORE INSERT` trigger on notices targeting reservation 12 that raises an error, then remove it after the request.

Set `RESERVATION_SERIES_UPDATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL that permits creating schemas. From `backend` in WSL, run the test below. It applies migrations `0001`–`0006`, starts the normal Go HTTP server, compares all captured cases, and exercises both forced failures and the lock schedules. With the URL unset, the database integration test is skipped.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestRecurringReservationUpdatePostgres$'
```

## Go recurring reservation deletion

Blazor sends `POST /api/Reservations/{id}/series-delete` with `originalReservation` and `scope`. The stored route ID determines both authorization and the targets. The owner and administrators can delete. Another member receives an empty `403`, an anonymous user receives an empty `401`, a missing numeric ID returns `404` Problem Details, and a nonnumeric route returns an empty `404`. The original snapshot still undergoes model validation. Omission yields the missing-name and missing-room errors on the default snapshot, and explicit null yields a required-field error. Snapshot start and end times are deserialized without applying booking business rules.

`single`, an omitted scope, an unknown string, and all scopes on non-recurring bookings use the same deletion as `DELETE /api/Reservations/{id}`. For recurring bookings, `all` and `following` reuse the series update lookup. A nonblank key is trimmed on the base, compared against stored keys, and restricted to the same organizer. The legacy lookup uses organizer ID, name, room, type, purpose, repeat type, repeat-end calendar day, and start and end times of day. `following` includes every matching row on the base calendar day. An empty result and a successful deletion both return an empty `204`. Existing notifications retain all values, including links to deleted bookings.

Go acquires the row locks in ascending ID order, resolves the selection again after waiting, and retries if new target rows require locks. It then takes the same ordered room/day locks as series edits and creates. One PostgreSQL statement deletes the locked targets in the same transaction. This avoids a lock-order inversion between edits and deletes and lets waiting creates use committed slot occupancy. The tests covered an edit expanding the subsequent deletion scope, a deletion followed by an edit returning `404`, a create accepted in a freed slot, a create rejected while the overlap still existed, and an adjacent create preserved after deletion. A forced failure on the second delete returned `500` and preserved every booking and notice in both databases. No schema change was needed.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/reservation_series_delete_csharp.json) records 24 cases. Reuse the reservation datasets from the [series update capture](../backend/internal/httpapi/testdata/reservation_series_update_csharp.json) in the disposable `d8fe3b7` setup above. Assign the fake direct room IDs `900001`, `900002`, and `900003` to users 1, 2, and 3 with a fake `IChatworkClient`. Before each case, clear bookings, notices, and Chatwork delivery logs, then seed the case's reservations, one notice per reservation, and one notice without a reservation target. Submit the recorded request and use its role's cookie and `Accept: application/json`. Compare the HTTP result, remaining reservation IDs and values, and preexisting notice rows. To reproduce the SQLite failure, raise an error in a `BEFORE DELETE` trigger for ID 12 once ID 10 has been removed within the transaction.

C# also attempted Chatwork delivery to the fake rooms on successful cancellations with recipients. See [Chatwork cancellation DMs](#go-chatwork-cancellation-dms) for Go delivery. Go uses its shared JSON diagnostics for conversion errors, omitting ASP.NET's trace ID and parser byte position.

Set `RESERVATION_SERIES_DELETE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL that permits creating schemas. From `backend` in WSL, run the command below. The test applies migrations `0001`–`0006`, starts the normal Go HTTP server, and runs the captured, failure, and concurrency cases. With the URL unset, the database integration test is skipped.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestRecurringReservationDeletePostgres$'
```

## Go Chatwork cancellation DMs

Both cancellation routes snapshot the deleted reservations while holding their existing PostgreSQL transaction locks. Delivery starts only after a successful commit with at least one deleted row. The representative is the first deleted row by date and ID. The Go sender reads the current names and `chatwork_direct_room_id` for the deduplicated organizer and participant IDs across the deleted rows. It uses the representative's room, date, time, label, and ID. Missing user records appear as participant ID labels where applicable and receive no delivery. For multiple rows the Japanese message includes the number deleted. The [C# HTTP and SQLite fixture](../backend/internal/httpapi/testdata/chatwork_reservation_cancel_csharp.json) records seven cases, including every fake room, form body, and delivery log. Its series case used reservation 71 as the representative even though the request addressed reservation 73 and another booking started earlier on the same date.

[Migration `0007`](../backend/migrations/0007_create_chatwork_delivery_logs.sql) stores the cancellation message, target, original scheduled start and delivery timestamps. It keeps the C# key `ReservationCanceled:reservation:{id}:user:{userId}` under a PostgreSQL unique constraint. This history allows canceled reservation IDs and users to be retained after their rows are removed. Preserve existing keys and statuses when importing history. When explicitly importing log IDs, reconcile the identity sequence before enabling writes.

Go inserts and commits the delivery claim before sending, using `ON CONFLICT DO NOTHING`. Every existing status suppresses another attempt. A missing room records `Skipped` without sending. A confirmed success records `Succeeded` and its sent time. An explicit HTTP rejection such as `503` records `Failed` and the status code. C# inserted results after HTTP and logged a timeout as `Failed`. Go leaves a claim `Unknown` during sending and whenever the response or result write cannot be confirmed. It stores neither provider response bodies nor tokens in delivery errors. A database claim failure suppresses HTTP. These delivery failures leave the committed cancellation response at `204`.

Automatic retries are disabled for all statuses. Reconcile an `Unknown` result against the provider or the recipient's room before considering an operator-approved resend. Keep its key during reconciliation to avoid duplicate DMs. A process exit after deleting but before claiming can leave no log, and exhausting the 10-second overall delivery budget can leave later recipients unclaimed. A crash between HTTP and the result write leaves the claim `Unknown`. This synchronous post-commit sender uses a 3-second HTTP timeout. A durable outbox could close the pre-claim gap and would need an explicit recovery policy for uncertain receipts.

Delivery defaults to disabled. Apply migration `0007` before setting `CHATWORK_ENABLED=true` and supply `CHATWORK_API_TOKEN` through the local environment. The default endpoint is `https://api.chatwork.com/v2/`. `APP_ENV=development` allows a loopback-only `CHATWORK_API_BASE_URL` override for synthetic testing. To reproduce the C# capture, run the disposable SQLite setup at `d8fe3b7`, point its typed `IChatworkClient` at a loopback fake instead of the configured Chatwork base address, and disable the reminder worker. Enable Chatwork with a synthetic token and seed the fixture's users and reservations. Have the fake return `503` for room `900003` and delay room `900002` beyond the C# client timeout for the failure case.

Set `CHATWORK_CANCELLATION_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. From `backend` in WSL, run the command below. It starts a loopback fake HTTP server, compares the C# fixture, checks committed database state inside the fake, and covers concurrent claims, failure at reservation commit, and replay after an uncertain result.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestChatworkReservationCancellationPostgres$'
```

## Go Chatwork creation DMs

`POST /api/Reservations` and `POST /api/Reservations/series` send creation DMs after the reservations and in-app notices commit. The representative is the first row by date and ID. Each existing user in the deduplicated union of organizers and participants across all created rows gets one delivery key, `ReservationCreated:reservation:{id}:user:{userId}`, using the representative ID. Users without a direct room get a `Skipped` log. The message uses the representative booking's details and includes the created count for a batch with multiple rows.

Room conflicts for allowed overlaps are captured under the existing room/day locks before any inserts. The series query deduplicates the existing rows by reservation ID across all submitted slots. Reading room conflicts after insertion could include the new rows or observe a different writer's changes. Series entries that overlap only within the same request do not appear in the existing-reservation section. Both room and participant overlap checks use strict time boundaries. The participant warning checks external appointments at the representative slot against the union from all occurrences, including the organizer, even when there are no in-app recipients. This lookup runs after commit, as in C#. Each warning shows up to five details and the full count. For equal-time participant conflicts, Go uses the same PostgreSQL collation order as in-app notices.

Creation uses the existing [Chatwork sender settings, migration `0007`, and retry policy](#go-chatwork-cancellation-dms). The shared sender claims the unique key before issuing an HTTP request. Delivery defaults to disabled. A `409` or a failed booking transaction produces no delivery attempts. An HTTP rejection records `Failed` with the status code and an unconfirmed response records `Unknown`. Either keeps the committed creation response at `201`. C# recorded the measured timeout as `Failed` and stored the provider response text on rejection. When notice persistence failed in the C# series case, three booking rows remained. Go rolled back all three along with the notices and made no HTTP calls.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/chatwork_reservation_create_csharp.json) records nine cases. Use the disposable `d8fe3b7` setup described for cancellation, point its Chatwork client at a loopback fake, and disable the reminder worker. Use the fixture's synthetic users, existing reservations, appointments, and request bodies. Start each case with empty reservations, notices, schedules, and delivery logs, then seed only its fixtures. For the failure case set room `900003` to return `503` and delay room `900002` beyond the client timeout. To reproduce the notice failure, raise an error in a SQLite `BEFORE INSERT` trigger on `Notifications`.

Set `CHATWORK_CREATION_TEST_DATABASE_URL` to a disposable PostgreSQL URL with schema creation privileges. From `backend` in WSL, run the test below. It compares the fake HTTP requests and database logs against the capture, checks committed bookings, notices and delivery claims from inside the fake handler, and verifies rollback and replay of a previously claimed creation event.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestChatworkReservationCreationPostgres$'
```

## Go Chatwork update DMs

`PUT /api/Reservations/{id}` sends update DMs using the addressed booking's before and after values. `POST /api/Reservations/{id}/series-update` selects the first affected row by its updated date and ID and uses that row's before and after values. The recipient IDs are the union of the representative's previous and current organizer and participant IDs. Removed participants can still receive a DM, and participants found only in other occurrences are outside this recipient set.

The message describes changes to the calendar date, room, category, time of day, purpose, and participant membership. The external appointment warning uses the representative's updated slot and current participants, including the organizer. The existing PostgreSQL collation orders equal-time participant names. If neither the tracked fields nor external appointment conflicts produce any details, there is no DM even if another series row changed. A warning can produce a DM without an edit to those fields. An empty target set produces no delivery. Both the single-route query flag and the series body flag `notifyParticipants=false` suppress in-app notices while the same Chatwork rules apply. The C# loopback runs verified these cases.

Go captures the before and after values under the existing locks and dispatches only after the update and in-app notices commit. Reading the booking again after commit could show a later writer's update. Go assigns one fresh change ID per committed update result and shares it across the recipients' keys `ReservationUpdated:reservation:{id}:user:{userId}:change:{changeId}`. Reprocessing the same result reuses the key. A separate committed edit receives a new change ID. This does not provide HTTP request idempotency.

The shared [Chatwork settings, delivery log and retry policy](#go-chatwork-cancellation-dms) apply, including default-disabled delivery, a unique claim before HTTP, and no automatic resend. An HTTP rejection records the status code and `Failed`. An unconfirmed response records `Unknown`. Neither changes the committed `204` response. C# recorded the fake timeout as `Failed` and retained the provider response text on rejection. When the notification write was forced to fail, C# kept the edited rows with no DM. Go rolled back the edits and notices together and made zero HTTP calls.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/chatwork_reservation_update_csharp.json) records 11 cases. Reuse the disposable `d8fe3b7` setup and loopback HTTP fake from [creation DMs](#go-chatwork-creation-dms). Seed the capture's synthetic users, original booking dataset, and appointments for each case. Clear bookings, appointments, notices and delivery logs between cases. For the failure case return `503` from room `900003` and delay room `900002` beyond the configured client timeout. A SQLite trigger that aborts a notification insert reproduces the C# persistence failure.

Set `CHATWORK_UPDATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. From `backend` in WSL, run the command below. It checks stored reservation values, notice counts, committed state from within the fake HTTP handler, message and log parity, replay of one event, and delivery for a second edit to the same reservation.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api -run '^TestChatworkReservationUpdatePostgres$'
```

## Go Chatwork reservation reminders

With the existing Chatwork setting enabled, the reservation worker runs an immediate pass at API startup and waits one minute after each completed pass. It uses the server's local wall time and selects existing reservations with `now < start <= now + 10 minutes`. It sends in ascending start order. Go breaks ties by reservation ID. C# only specifies start order. Each occurrence is processed independently, including recurring bookings. The organizer and participants are deduplicated by user ID and the message uses the selected occurrence's details. A deletion committed before selection excludes the row. Like C#, a pass uses a snapshot of its selected rows, so an edit or deletion after selection can race with a send.

The [C# HTTP and SQLite capture](../backend/internal/httpapi/testdata/chatwork_reservation_reminder_csharp.json) records the fixed wall clock, synthetic users and reservations, and actual loopback HTTP requests and delivery logs. It includes a booking set through a C# PUT with an explicit `+09:00` offset. SQLite reloaded the stored start as a zone-free .NET `DateTime`. Go uses the same key, `Reminder10Minutes:reservation:{id}:user:{userId}:start:{scheduledStartTime:O}`, with exactly seven fractional digits and no time-zone suffix. For example, the exact ten-minute boundary uses `2026-06-22T09:10:00.1234567`. Both zero and trailing fractional digits are significant in the key. Use the same server time zone as the C# booking calendar.

The worker reuses the [Chatwork settings, client and unique delivery claims](#go-chatwork-cancellation-dms). Every existing status prevents another attempt for its key. A changed start changes the key. A user with no direct room is logged as `Skipped`. The measured `503` and timeout produced C# `Failed` rows. Go records the HTTP status for the `503` and leaves an unconfirmed send `Unknown`. There is no automatic resend. Shutdown cancels the worker's in-flight HTTP and DB operations and joins it before the PostgreSQL pool closes. An interrupted attempt retains its committed `Unknown` claim. The worker stays disabled with the default configuration.

[Migration `0008`](../backend/migrations/0008_index_reservation_start_times.sql) adds a start-time and ID index so the scheduled range can be read in order. A repeated full-table read would inspect historical rows on every pass. PostgreSQL 16.15 picked an index scan for the ten-minute range with 50,000 synthetic rows. Each pass releases its query connection before sending HTTP and holds no reservation row lock during delivery.

To reproduce the C# capture, use the disposable `d8fe3b7` SQLite and loopback Chatwork setup above. Disable its hosted reminder registration in the temporary copy and invoke `ChatworkReminderWorker.ProcessUpcomingRemindersAsync` once per pass with the fixture's `now` instead of using the real clock. Seed the recorded users and reservations, remove the case's deleted IDs, and preload the first log in the existing-key case. For the failure case return `503` for room `900003` and delay room `900002` past the C# HTTP timeout. Both polls of each case must preserve the first pass's delivery log count and HTTP calls.

Set `CHATWORK_REMINDER_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. Run from `backend` in WSL. The PostgreSQL test compares the C# keys byte for byte, verifies repeated and concurrent passes, a changed start time, and actual Go API startup, restart, and shutdown. The scheduler test uses a fake clock to verify the one-minute delay without a real-minute wait.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api ./internal/chatwork \
  -run '^(TestChatworkReservationReminderPostgres|TestReminderLoopRunsImmediatelyAndWaitsAfterEachPass)$'
```

In WSL, the fixed-time C# window selected four reservations in order and produced seven HTTP calls and one skipped room. The PostgreSQL 16.15 reminder test passed all eight subcases, and all 18 PostgreSQL integration tests passed with the database URLs set. Without the URLs, `go test ./...` passed with those 18 tests skipped. `go vet ./...` and `git diff --check`, including the new files, passed.

## Go Chatwork external appointment reminders

The [existing reminder worker](#go-chatwork-reservation-reminders) processes meeting room reservations before work schedules, as in C#. Both passes use the same server-local clock. The worker runs immediately at startup and waits a minute after both passes finish. The work schedule pass selects `ExternalAppointment` rows with both times set and `now < start <= now + 10 minutes`. It processes each occurrence in start order, using ID as the tie-breaker in Go. C# specifies start order only. Rows deleted before the poll are excluded. An edit or deletion after selection can race with delivery in either implementation.

Positive participant IDs are deduplicated in their stored order. The creator receives a DM only if their ID is listed. Missing users receive no DM. The message uses the stored `Participants` text as-is when it contains a non-whitespace character, including its leading and trailing whitespace. A blank value displays `未設定`. This text is independent of the recipient IDs.

The [C# capture](../backend/internal/httpapi/testdata/chatwork_work_schedule_reminder_csharp.json) records three fixed-time SQLite cases, their loopback requests and logs. The key is `WorkScheduleReminder10Minutes:work-schedule:{id}:user:{userId}:start:{scheduledStartTime:O}`. Reloading start times from SQLite produced seven fractional digits and no timezone suffix. For example, the ten-minute boundary key ends in `2026-06-22T09:10:00.1234567`, and a whole second uses `.0000000`. A work schedule log stores `reservation_id=0` and the corresponding `work_schedule_entry_id`. A reservation and a work schedule with the same numeric ID have distinct keys and each produced its own DM in the same pass.

The existing [Chatwork settings, HTTP client and unique claims](#go-chatwork-cancellation-dms) apply. Delivery defaults to disabled. Existing keys suppress retries across restarts and concurrent passes. Changing the start generates a new key. C# recorded the measured HTTP `503` and timeout as `Failed`. Go records the HTTP status for the rejection and leaves an unconfirmed send `Unknown`, including when shutdown interrupts delivery. The shared worker cancels the active HTTP request and joins before the database pool closes. The existing reconciliation policy applies.

[Migration `0009`](../backend/migrations/0009_index_work_schedule_reminder_start_times.sql) adds a partial index on start time and ID for timed external appointments. It keeps other work schedule types out of the poll index. PostgreSQL 16.15 used it for the reminder query over 50,000 synthetic rows with mixed appointment types. The polling query releases its connection before issuing HTTP requests.

To reproduce the C# capture, use the temporary `d8fe3b7` SQLite and loopback setup described for reservation reminders. Insert the fixture's users, work schedules and any seeded room reservation. Remove the recorded deleted IDs and preload the first log in the existing-key case. In the failure case, return `503` from fake room `900003` and delay room `900002` beyond the C# timeout. Invoke `ChatworkReminderWorker.ProcessUpcomingRemindersAsync` twice with the captured `now` and compare both pass results.

Set `CHATWORK_WORK_SCHEDULE_REMINDER_TEST_DATABASE_URL` and `CHATWORK_REMINDER_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. Run from `backend` in WSL. The tests verify the SQLite fixtures, replay and concurrent claims, rescheduling, real API startup and restart, and cancellation during HTTP delivery. The existing scheduler test covers the elapsed minute between passes.

```bash
TZ=Asia/Tokyo go test -v -count=1 ./cmd/api ./internal/chatwork \
  -run '^(TestChatwork(Reservation|WorkSchedule)ReminderPostgres|TestReminderLoopRunsImmediatelyAndWaitsAfterEachPass)$'
```

The C# window produced five work schedule HTTP requests, one skipped work schedule room, and one reservation HTTP request. All eight new PostgreSQL subcases and all 19 PostgreSQL integration tests passed in WSL. With the database URLs unset, `go test ./...` passed with the 19 integration tests skipped. `go vet ./...` and `git diff --check` passed.

## Go single work schedule creation

`POST /api/work-schedule-entries` uses session authentication and the existing write origin protection. It returns `201` with the work schedule list's JSON shape and an absolute `Location` pointing to `GET /api/work-schedule-entries/{id}`. Any signed-in user can read the detail. A missing numeric ID returns a `404` problem response. The creator ID comes from the session. Positive participant IDs retain their input order after deduplication, and the creator is prepended when absent. All participant IDs must exist. A blank display name list is built from the users' names in that order.

External appointments require a title and both times. The times retain their time of day and take the selected date at 100 ns precision. Numeric UTC offsets are converted to server local time during JSON binding, so run both backends in the same timezone. Work from home defaults to `在宅` and discards times and the leave period. Leave defaults to `休暇`, requires a leave period, and discards times. The single route rejects a nonblank repeat mode other than `しない`. An external appointment with only a repeat end date is accepted and stored without recurrence, as in C#. The other schedule types reject a repeat end date on this route.

All participants, including the creator, receive an in-app notification. An external appointment still returns `201` when a participant has an intersecting room reservation or another external appointment on that day. A shared endpoint alone does not overlap. In the [captured overlap case](../backend/internal/httpapi/testdata/work_schedule_create_csharp.json), both recipients received a `Warning` with the same five details and two additional conflicts in the summary. The DM uses the same conflict snapshot. Go uses Unicode root collation for ordering tied warning details by participant name, matching the tested names under C#'s reference culture. A different C# runtime culture may order tied names differently.

Go inserts the work schedule and its in-app notifications in one transaction. The C# SQLite capture retains the schedule when the later notification save fails. Go rolled back both and returned `500` with no DM. A shared conflict snapshot prevents a concurrent write from giving the in-app and DM versions different warnings. Conflicts are advisory, so this query does not take scheduling locks. Go serves its standard problem details for model binding and omits ASP.NET trace IDs and parser line/byte positions.

Chatwork starts after commit using the [existing configuration, loopback test client and unique delivery claims](#go-chatwork-cancellation-dms). It defaults to disabled. Each participant uses `WorkScheduleCreated:work-schedule:{id}:user:{userId}`, with `reservation_id=0` and the created ID in `work_schedule_entry_id`. The scheduled start is the normalized start time or the day at midnight for an untimed schedule. Missing rooms are logged as `Skipped`. Both backends returned `201` when the fake rejected or timed out. Go records HTTP `503` without the provider body and records the uncertain timeout as `Unknown`, compared with C#'s `Failed`. Existing keys suppress further attempts, including uncertain results. The reconciliation policy for prior claims applies.

[Migration `0010`](../backend/migrations/0010_enable_work_schedule_creation.sql) uses a PostgreSQL identity in place of application-side ID allocation. `GENERATED BY DEFAULT` keeps explicit IDs available for imports. It seeds the sequence above existing rows and references in notices and delivery logs, including references to deleted schedules. The PostgreSQL test applied it after seeding a schedule with ID `7000` and historical references `7100` and `7200`. The old row remained and the next insert received `7201`. Apply it after importing those rows. From `backend` in WSL, run `psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0010_enable_work_schedule_creation.sql` on the local database. If importing after migration, advance the sequence before enabling writes and carry forward the SQLite high-water mark for deleted IDs that no longer have references.

To reproduce the C# capture, use a temporary checkout of `d8fe3b7` and a fresh SQLite file. Set `TZ=Asia/Tokyo`, `ASPNETCORE_ENVIRONMENT=Development`, `Database__EnsureCreatedOnStartup=true`, `Database__ApplyMigrationsOnStartup=false` and `RoomConflictDetection__Enabled=false`. Set `ConnectionStrings__DefaultConnection` to the temporary file. Point the temporary C# Chatwork client's base address in `Program.cs` to a loopback `/v2/` fake, disable the hosted reminder worker in that copy, and set `Chatwork__Enabled=true` with a synthetic token. The fake should record the room and URL-encoded `body` field and return the case's `fakeResponses`. Use a delay longer than the client's timeout for room `900002` in the failure case.

Register the fixture users in ID order and seed their direct rooms. For each case, clear reservations, schedules, notifications and delivery logs, and load the fixture's `seeds` on 2026-06-22. Send each payload with `Content-Type: application/json`, `Accept: application/json`, and the indicated session. Follow the successful `Location` as another signed-in user. Compare schedule rows, notification targets and delivery keys by generated ID. Use an SQLite `BEFORE INSERT` trigger on `Notifications` to force the save error for `notice_failure`. The fixture replaces the development exception stack with a marker.

In WSL, set `WORK_SCHEDULE_CREATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. From `backend` run:

```bash
go test -v -count=1 ./cmd/api -run '^TestWorkScheduleCreationPostgres$'
go test ./...
go vet ./...
```

The integration test applies migrations in an isolated schema, starts the API and uses a loopback fake to inspect committed schedules, notices and delivery claims before receiving each DM. It reproduces 28 C# POST cases and the detail GET checks, and covers origin protection, rollback, replay and disabled delivery. All 20 PostgreSQL integration suites passed on PostgreSQL 16.15. With the database URLs unset, `go test ./...` passed with those suites skipped. `go vet ./...` and `git diff --check` passed.

## Go recurring work schedule creation

`POST /api/work-schedule-entries/series` uses the single-entry route's JSON binding, session authentication, and write origin protection. Its success response is a `200` array. Requests without recurrence use single-entry creation and return a one-element array with single-entry notifications and delivery keys. The route has no `Location` header. It preserves the series route's trim-and-dispatch behavior, including the single-entry validation of a padded `" しない "`.

Only external appointments can recur. Daily recurrence steps by one day and weekly recurrence steps by seven days from the original weekday. Both include an eligible end date and skip Saturdays and Sundays. A weekend weekly anchor stays on that weekday and produces zero occurrences. The route rejects an empty result or more than 60 occurrences. It stops expanding at the 61st eligible date and can include `9999-12-31`. The C# capture returned `500` on that last date when the iterator tried to advance past `DateTime.MaxValue`.

Each occurrence has an identity ID allocated by migration `0010`, shares a lowercase 32-digit UUIDv4 series ID, and uses the requested time of day on its own date. The response is ordered by date, start time, and ID, with one occurrence per date. For example, daily recurrence from Friday June 19 to Tuesday June 23, 2026 writes three rows for June 19, 22, and 23 with the same series ID. Existing list and detail reads return these stored values.

All occurrences use the same normalized participants. One in-app notice and one DM attempt per participant reference the first saved occurrence. A single conflict query after insertion covers all occurrence dates and excludes each slot's own ID. It supplies one snapshot to both message formats, including overlaps that only occur later in the series. The measured warning contains the first five conflicts in date and start order and a count of the rest. The messages use the period between the first and last saved dates, the recurrence type, and the occurrence count. The DM also includes the time range.

Go saves every occurrence and the in-app notices in one transaction. The [C# capture](../backend/internal/httpapi/testdata/work_schedule_series_create_csharp.json) returned `200`, kept three rows, and attempted DMs when the summary notification save failed. Go returns `500` and rolls back all rows without sending. A forced second INSERT failure rolls back all rows in both implementations. Keeping the external send after commit lets Go preserve `200` when the DM provider fails.

The existing Chatwork settings default delivery to disabled. Each series delivery uses `WorkScheduleSeriesCreated:work-schedule-series:{seriesId}:user:{userId}`, `reservation_id=0`, and the representative row ID and start time. The unique claim prevents another attempt on replay. An HTTP `503` records a sanitized failure and an uncertain timeout keeps the existing `Unknown` behavior. C# recorded both failures as `Failed`.

To reproduce the C# capture, use the temporary `d8fe3b7` SQLite and loopback Chatwork setup for [single work schedule creation](#go-single-work-schedule-creation). Register the synthetic users in fixture ID order and set their direct rooms. Clear schedule, reservation, notification, and delivery rows before each case, then apply its dated `seeds` and `fakeResponses`. POST its `request` body to `/api/work-schedule-entries/series` with the indicated session and `Accept: application/json`. Compare the array to the SQLite rows by date, start time, and ID. Follow the list and detail reads for the saved ID range and verify the series key and the representative ID in the notices and logs. For `notice_failure`, make a SQLite `BEFORE INSERT` trigger on `Notifications` fail for user 3. For `mid_series_insert_failure`, make the same kind of trigger abort the `WorkScheduleEntries` insert for June 22, the second occurrence. The capture includes 25 POST cases. It stores the matching successful POST arrays and SQLite rows once and replaces development exception stacks with markers.

In WSL, set `WORK_SCHEDULE_SERIES_CREATE_TEST_DATABASE_URL` and `WORK_SCHEDULE_CREATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. Run from `backend`:

```bash
go test -v -count=1 ./cmd/api -run '^TestWorkSchedule(Series)?CreationPostgres$'
go test ./...
go vet ./...
```

The integration test creates and drops an isolated schema, applies migrations through `0010`, and starts a loopback Go API and a fake Chatwork server. It checks all saved entries through the list and detail routes and verifies committed rows, notices, and delivery claims from inside the fake HTTP handler. The date expansion unit tests run without a database. All 21 PostgreSQL integration suites passed on PostgreSQL 16.15, including single-entry creation. The database-free `go test ./...` passed with those suites skipped. `go vet ./...` and `git diff --check` passed.

## Go single work schedule update

`PUT /api/work-schedule-entries/{id}` returns `200` and the saved entry in the same shape as the detail route. The existing session and write-origin checks apply. Only the stored creator and an administrator can edit. Other signed-in users get `403`, missing numeric IDs get `404`, and unauthenticated requests get `401`. Shared request binding runs before the service. As in C#, the recurrence checks run before looking up the row, and schedule and participant validation run after checking ownership. The stored creator is used when normalizing participants, including edits by an administrator.

Go uses a PostgreSQL row lock to read the previous values and derives all changes from that snapshot. This serializes edits to the same entry and uses the committed values of a preceding edit. It retains the ID, creator, creation time, series ID, recurrence type and recurrence end date, and updates the modification time. Only the selected row is edited, including when it belongs to a series. The integration test held the row lock, changed the title, and verified that the waiting request quoted that changed title as its previous value.

In-app notifications are written for added, removed and retained participants in that order. Chatwork attempts retained, added and removed recipients in that order. Added participants use the new date and details and receive any new overlap warning. Removed participants receive the prior title and date, without a warning, and their delivery log uses the previous scheduled start. Retained participants receive a notice and DM for recognized changes or a current warning. A no-op or a change confined to the participant display text or order creates no delivery unless there is a warning. The shared conflict query excludes the edited row and excludes overlaps that only touch at a time boundary. It supplies the same current conflict snapshot to both notification channels.

The [reference capture](../backend/internal/httpapi/testdata/work_schedule_update_csharp.json) covers 27 PUT cases. An in-app notice is reused when the exact message already exists for the same user, work schedule ID and notice type. A matching newest notice keeps its ID, gets the current target date and timestamp, and becomes unread. An exact match triggers deletion of older notices in that scope. C# also does this when only an older notice matches, which leaves a newer, different notice intact with its original read state and timestamp. Go reproduces both cases. Without an exact match, a new row is inserted and existing messages remain.

Go saves the schedule and notices in one transaction. In the forced-notice failure case, C# returned `500` with its edit still saved. Go returned `500` and rolled back the edit. Neither sent a DM. The Go test also checks the edit and notices from the fake Chatwork handler before letting a send proceed. That verifies the HTTP send happens after commit and that the notice failure cannot emit a DM.

Each delivery uses `WorkScheduleUpdated:work-schedule:{id}:user:{userId}:change:{changeId}` with a single lowercase UUIDv4 change ID for the whole event. The existing unique delivery claim suppresses a replay of that event. A separate committed update gets a fresh change ID even when the warning text is identical. Delivery defaults to disabled. For the mock HTTP `503` and timeout, both backends returned `200`. Go stores a sanitized HTTP error and keeps an uncertain timeout `Unknown`. C# stored both as `Failed`.

To reproduce the C# capture in WSL, use the disposable `d8fe3b7` SQLite and loopback Chatwork setup for [single work schedule creation](#go-single-work-schedule-creation). Register the six synthetic users in fixture order, using the first account to register the rest, and set their direct rooms. Before each case, reset the four fixture tables and seed `initialEntries`, the case's dated `seeds` and existing notices from `initial`. Send the case's `request` to the indicated PUT ID and use the recorded actor's session and `Accept: application/json`. Successful PUT bodies matched the saved row and detail response. The fixture records that row once and replaces the development exception stack with a marker. For `notice_failure`, make a SQLite `BEFORE INSERT` trigger on `Notifications` abort for user 3. For `provider_failure_and_timeout`, return `503` for room `900003` and delay room `900002` past the client timeout.

In WSL, set `WORK_SCHEDULE_UPDATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. From `backend` run:

```bash
go test -v -count=1 ./cmd/api -run '^TestWorkScheduleUpdatePostgres$'
go test ./...
go vet ./...
```

The integration test creates and drops an isolated schema, applies migrations through `0010`, and starts the Go API and fake Chatwork server on loopback. It compares the capture and checks origin protection, the concurrent row lock, replay, distinct change IDs, and disabled delivery. All 22 PostgreSQL integration suites passed on PostgreSQL 16.15. The database-free `go test ./...` passed with those suites skipped. `go vet ./...` and `git diff --check` passed.

## Go recurring work schedule update

`PUT /api/work-schedule-entries/{id}/series?scope=...` uses the existing session and write-origin checks. Scope trims whitespace and folds to lowercase. Omitted and blank values dispatch to `single`, which uses ordinary single-entry update and its notification and delivery keys. Each of `single`, `following`, and `all` returns an empty `204` on success. An unknown scope returns `400`.

The batch scopes select external appointments with an exact stored series ID matching the trimmed key of the selected row. `following` starts at the selected calendar day and start time. It includes equal start times and later dates, and uses midnight when a start time is absent. A single or non-external selected entry, an empty target set, and a request to change recurrence conditions return `400`. For example, a selected legacy row with a padded key and no exact-key rows has an empty target set. If exact-key rows exist, the padded row itself stays outside that set. The caller must own every target or be an administrator.

Go locks the selected row and candidates together in ID order. After the locks are acquired it resolves the range again using the locked selected row. If the scope gained an unlocked target, it restarts the transaction so all targets are locked in the same order. Permissions are checked against the locked rows. This handles edits to the selected date, time, or type while waiting without taking the selected row out of order. The integration test used a concurrent single PUT to move the selected start earlier and verified that the batch included the newly eligible row. A separate lock test verified a `403` when a target's creator changed during the wait.

Each target keeps its date, ID, creator, creation time, series ID and recurrence values. The edit updates the title, times, participants and modification timestamp. Times are attached to each target's own calendar day, and participant normalization uses that target's creator. The requested date has no effect on batch scopes.

In-app notices and DMs go once to the union of participant IDs from before and after all selected occurrences. The first updated row by date, start time, and ID is the reference. The summary includes scope, period, count, time and the representative's participant display. It compares the first row of each ordered snapshot for title, time and participant ID changes. C# still generates a summary when those values are unchanged. Go reads overlaps in one query over the updated occurrences using each row's participants, and both messages use the resulting warnings. The first five distinct participant/source pairs are displayed. The remaining-count suffix follows C# and counts raw conflicts before deduplication, which matters for multiple same-day occurrences.

Go saves all changes and notices in one PostgreSQL transaction and sends DMs after it commits. The C# notification failure case returned `204`, kept the updates, and attempted DMs. Go returned `500`, rolled back every edited row and sent zero DMs. A failure on the last row update rolled back both implementations. Both return `204` after a post-commit delivery failure. The HTTP `503` and timeout produced C# `Failed` rows. Go sanitizes the HTTP error and retains the existing `Unknown` claim for the timeout. Chatwork defaults to disabled.

The delivery key is `WorkScheduleSeriesUpdated:work-schedule-series:{seriesId}:scope:{scope}:user:{userId}:change:{changeId}`. All recipients share a lowercase UUIDv4 change ID for the event. The log keeps `reservation_id=0`, the representative's ID and its updated scheduled start. The existing unique claim suppresses replay. Single-scope updates use the ordinary single-entry event key.

To reproduce the [26-case C# capture](../backend/internal/httpapi/testdata/work_schedule_series_update_csharp.json), use the disposable `d8fe3b7` SQLite and loopback Chatwork setup for [single work schedule creation](#go-single-work-schedule-creation). Register the synthetic users in fixture order, using the administrator to register the remaining users, and set their recorded direct rooms. Before each request clear work schedules, reservations, notifications and delivery logs. Load the named `dataset` and the case's `seeds`. PUT the recorded payload, ID and scope with the indicated session and `Accept: application/json`. For `notice_failure`, abort a SQLite `BEFORE INSERT` trigger for notices to user 3. For `last_update_failure`, abort a `BEFORE UPDATE` trigger for schedule 53. For the provider failure case, return `503` from room `900003` and delay room `900002` beyond the client timeout. Compare all stored rows and the requests, notices and logs in recipient order, allowing for generated timestamps and change IDs.

In WSL, set `WORK_SCHEDULE_SERIES_UPDATE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. From `backend` run:

```bash
go test -v -count=1 ./cmd/api -run '^TestWorkScheduleSeriesUpdatePostgres$'
go test ./...
go vet ./...
```

The integration test uses a temporary schema and the normal Go API with a loopback Chatwork fake. It verifies the captured responses, rows, messages and logs, origin protection, concurrent edits, replay, and disabled delivery. All 23 PostgreSQL integration suites passed on PostgreSQL 16.15. The database-free `go test ./...` passed with those suites skipped. `go vet ./...` and `git diff --check` passed.

## Go single work schedule deletion

`DELETE /api/work-schedule-entries/{id}` reuses the session, origin and numeric ID handling. The stored creator and administrators can delete the addressed entry. A successful deletion returns an empty `204`. Missing or previously deleted numeric IDs return `404` Problem Details. Invalid ID paths return an empty `404`, unauthenticated requests return `401`, and other signed-in users return `403`. Other occurrences in the same series remain available.

Go takes the same PostgreSQL row lock as single-entry update. Authorization, participant IDs and all notification fields come from the locked row. In PostgreSQL, a delete waiting behind an update used the committed new title, date and participants. An update waiting behind a delete received `404`. Two concurrent deletes produced one `204`, one `404`, and one set of notices and delivery claims. The owner check also used the owner committed while the delete waited.

The `Info` notice goes to the positive participant IDs in stored order after deduplication. The creator receives it when included in that list. The notice body contains the deleted schedule's type, title and calendar date, and the row stores the full original date value and the deleted ID. The copied ID remains available as a notification reference after the entry is removed. An existing exact message for the same user, work schedule ID and type triggers the shared notice reuse rules. When the newest row matches, it retains its ID, gets a fresh timestamp and becomes unread. If only an older row matches, C# keeps the newest different message intact and removes the older rows in that scope. Without a match it adds a row and keeps the history. Other recipients, notice types and targets keep their existing records.

Go saves the delete and in-app notices in one transaction. In the forced notice failure case, C# returned `500` after committing the delete. Go returned `500` with the schedule still present. Both produced zero DMs. A single transaction makes a failed notice write retryable while retaining the schedule. Delivery begins after commit and the fake verifies that the schedule is already absent and the notices are visible before receiving HTTP.

The shared Chatwork sender uses `WorkScheduleDeleted:work-schedule:{id}:user:{userId}`, `reservation_id=0` and the copied work schedule ID. Its scheduled start is the stored start or calendar midnight for untimed entries. The message uses the stored participant text, includes the time range for external appointments or the leave period for leave, and shows `未設定` for blank participant text. Positive IDs without a user row still receive an in-app notice and are omitted from DM delivery. A user with no direct room is logged as `Skipped`. Delivery defaults to disabled. Existing keys suppress replay. The mock rejection and timeout left both backends at `204`. Go logs a sanitized `503` failure and retains `Unknown` for an uncertain timeout. C# records both as `Failed`.

To reproduce the [C# capture](../backend/internal/httpapi/testdata/work_schedule_delete_csharp.json), use the temporary `d8fe3b7` SQLite and loopback setup for [single work schedule creation](#go-single-work-schedule-creation). Register the synthetic users in fixture order and assign the recorded DM rooms. For each case clear schedules, notices and delivery logs, then seed the named dataset, `initial` notices and `existingLogs`. Issue the DELETE with the recorded session and `Accept: application/json`. Repeat the creator case to check the second `404`. To force the notice failure, abort a SQLite `BEFORE INSERT` trigger on `Notifications` for user 2. Return `503` for fake room `900003` and delay `900002` past the fake client's timeout in the provider failure case. The fixture replaces the development exception stack with a marker.

In WSL, set `WORK_SCHEDULE_DELETE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. Run from `backend`:

```bash
go test -v -count=1 ./cmd/api -run '^TestWorkScheduleDeletionPostgres$'
go test ./...
go vet ./...
```

The Go integration test uses the existing migrations and an isolated schema. It compares 18 C# cases plus the repeat DELETE and checks origin protection, row-lock contention, replay, and the disabled default. All 24 PostgreSQL integration suites passed on PostgreSQL 16.15. The database-free `go test ./...` passed with those suites skipped. `go vet ./...`, formatting, and `git diff --check` passed.

## Go recurring work schedule deletion

`DELETE /api/work-schedule-entries/{id}/series?scope=...` uses the existing session, origin and numeric ID handling. Scope is trimmed and lowercased. Omitted, blank and explicit `single` scopes use single-entry deletion, including its notice and Chatwork key. Successful requests return an empty `204`. Invalid scope returns `400` with `削除範囲が不正です。`. A selected entry outside an external appointment series returns `繰り返し社外予定のみまとめて削除できます。`, and an empty target set returns `削除対象の社外予定がありません。` with the same status. A missing selected ID returns `404`. An unauthorized target makes the whole batch return `403`.

Batch deletion and update use the same target lookup and lock transaction. The selected row is included in the ID-ordered locks, even when its padded series key does not match the stored target keys. `following` includes later days and entries at or after the selected start on the same day, using midnight for a missing start. `all` includes all external appointments with the exact key. After waiting, Go resolves the scope again and checks ownership on every locked target. A newly eligible ID restarts the transaction to acquire the enlarged set in order. A prior single deletion can shrink the set. Update continues to map target errors to its original edit messages.

The rows are ordered by date, start time or stored date when untimed, and ID. The first row supplies the title, date, copied ID and stored participant display for the summary. Positive participant IDs across the deleted entries are deduplicated in that order. Each receives one `Info` summary under the existing notice reuse rules. The notice and DM show the scope, period, count, and representative's participant display, with `未設定` for blank text. The DM also shows the representative's time range or `終日` when untimed. The existing schema preserves references and history for the deleted IDs.

Go deletes all locked targets and persists their notices in one PostgreSQL transaction. A forced second delete failed with `500`, retained both rows, and sent zero DMs in both databases. When notice persistence failed, C# returned `204`, left both rows deleted, and attempted delivery. Go returned `500`, restored both rows, and sent zero DMs.

After commit, Chatwork uses `WorkScheduleSeriesDeleted:work-schedule-series:{seriesId}:scope:{scope}:user:{userId}`. The log stores the representative's ID and its start, or the representative date at midnight if untimed, with `reservation_id=0`. The existing delivery claim suppresses replay. A missing direct room yields `Skipped`. The mock `503` and timeout left both backends at `204`. Go logs a sanitized `503` failure and keeps the uncertain timeout `Unknown`. C# logged both as `Failed`. Delivery defaults to disabled.

To reproduce the [22-case C# capture](../backend/internal/httpapi/testdata/work_schedule_series_delete_csharp.json), use the temporary `d8fe3b7` SQLite and loopback Chatwork setup for [single work schedule creation](#go-single-work-schedule-creation). Register the synthetic users in fixture order and assign their recorded direct rooms. Before each case clear schedules, notices and delivery logs, then load the named `dataset`, `initial` notices and `existingLogs`. DELETE the recorded ID and scope with the indicated session and `Accept: application/json`. Repeat `following_boundary` to check the second `404`. For `second_delete_failure`, use a SQLite `BEFORE DELETE` trigger on row 53 that aborts when row 52 has already been removed inside the transaction. For `notice_failure`, abort a SQLite `BEFORE INSERT` trigger on `Notifications` for user 3. For the provider failure case, return `503` for room `900003` and delay room `900002` beyond the client timeout. Compare remaining rows, notices, mock requests and delivery logs.

In WSL, set `WORK_SCHEDULE_SERIES_DELETE_TEST_DATABASE_URL` to a disposable local PostgreSQL URL with schema creation privileges. Run from `backend`:

```bash
go test -v -count=1 ./cmd/api -run '^TestWorkScheduleSeriesDeletionPostgres$'
go test ./...
go vet ./...
```

The Go integration test applies the existing migrations in a temporary schema and starts the normal API and loopback fake. It compares the capture and exercises scope expansion behind a series edit, scope shrinkage behind a single delete, authorization after a lock wait, rollback, post-commit delivery and replay. All 25 PostgreSQL integration suites passed on PostgreSQL 16.15. The database-free `go test ./...` passed with those suites skipped. `go vet ./...`, formatting, and `git diff --check` passed.

## Recovery to C# and SQLite

Rehearse each stage in development with synthetic rows and a recording fake Chatwork sender. `scripts/deploy/deploy-prod.sh` uses SQLite's backup API and integrity check. `scripts/deploy/rollback-prod.sh` restores the earlier API, web assets and final SQLite snapshot. A rollback after Go accepts writes also needs the committed PostgreSQL state.

| Stage | Data and action for returning to C# | Passing rehearsal |
| --- | --- | --- |
| Before cutover | Keep C#/SQLite authoritative. Retain matching API/Blazor builds, configuration, cookie-protection keys and a consistent SQLite backup with schema history. Prepare a reversible mapping for every table in `MeetingRoomBooker.Api/Data/AppDbContext.cs`. | Restore to an isolated C# instance. Run SQLite integrity checks, compare the reconciliation fields below, test login/remember-me and role-restricted actions, then perform sample booking and schedule edits. Record restore time. |
| While writes are stopped | Block incoming writes, drain requests/sends and stop both workers. Account for auth endpoints that can update users. Take and identify the final consistent SQLite snapshot. Import to PostgreSQL with delivery disabled and retain the untouched snapshot. | Compare both stores at the same cutoff. On any discrepancy, discard the candidate, restore the final snapshot and matching C# assets/config, verify reads and roles, then enable only the C# writer and workers. Prove no accepted write occurred after the cutoff. |
| After Go resumes writes | Freeze and drain Go, its workers and sends again. Snapshot PostgreSQL at the final committed transaction. Reverse-export its complete state into a fresh copy of the C#-compatible schema, preserving IDs, original delivery keys and deleted-row absence. Import rows directly with side effects disabled. | Under Go, create, change and delete bookings and work schedules (including series), change membership, read/delete notices and update a conflict record. After reversal compare every surviving row and deleted ID against PostgreSQL. Test C# login, next-ID allocation, list/detail and edits. Keep writes frozen on mismatch. Enable only C# after reconciliation. |

**Data reconciliation.** Compare counts and canonical row values by primary key across Users, Reservations, WorkScheduleEntries, Notifications, ChatworkDeliveryLogs and RoomConflictRecords. Check password-hash compatibility, admin flags, calendar dates/times, nulls, enum values, participant ID lists (comma-separated in SQLite), series IDs including legacy nulls, notification targets/read flags and conflict detection keys. Carry forward ID allocation high-water marks, including IDs of deleted rows.

**Notification rehearsal.** Preserve each delivery's recipient, target, type, key, status, attempt/sent times and scheduled start. The direct-send services consult existing keys for all statuses, and update keys contain a fresh change ID (`MeetingRoomBooker.Api/Services/Chatwork/ChatworkDeliveryKeys.cs`). Replaying write APIs would regenerate side effects. At a reminder boundary, run the fake sender through cutover, rollback and worker restart. Assert one receipt per expected event/recipient, zero resends for logged keys and the expected in-app rows. Simulate interruption between send and log commit. Reconcile ambiguous receipts before resuming delivery.

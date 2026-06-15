# MeetingRoomBooker

MeetingRoomBooker is an internal meeting room booking and employee schedule management system built with **Blazor WebAssembly**, **ASP.NET Core Web API**, **Entity Framework Core**, and **SQLite**.

It was originally developed to solve a real internal operations problem: meeting room reservations were managed in a shared calendar where room bookings and unrelated schedules were mixed together, which made availability hard to understand and caused duplicate reservations.

This project focuses not only on basic CRUD operations, but also on practical operational concerns such as:

* API-side conflict validation
* participant-based schedule conflict warnings
* recurring reservation handling
* in-app notifications
* Chatwork direct notifications
* reminder delivery
* admin-controlled user management
* safe deployment and database migration practices

---

## Background

Before MeetingRoomBooker, meeting room usage was managed through a general-purpose calendar. This created several operational issues:

* meeting room reservations were mixed with non-room schedules
* users could miss updates or cancellations
* responsibility for confirming changes was unclear
* duplicate bookings were sometimes noticed too late
* reception and reminder workflows depended on manual coordination

MeetingRoomBooker was designed as a dedicated workflow for small internal teams, bringing reservations, participants, notifications, conflict checks, and operational visibility into one system.

---

## Key Features

### Meeting room reservations

* Create, update, and delete meeting room reservations
* Select date, time, room, category, purpose, and number of attendees
* Add participants to reservations
* View reservations on calendar and timeline screens
* Join or leave existing reservations
* Restrict edits and deletes to organizers or admins
* Validate room/time conflicts on the API side

### Recurring reservations

* Create daily and weekly recurring reservations
* Update recurring reservations by scope:

  * single occurrence
  * this and following
  * entire series
* Delete recurring reservations by scope:

  * single occurrence
  * this and following
  * entire series
* Keep recurring reservation logic on the backend so the UI does not become the source of truth

### Employee work schedules

MeetingRoomBooker also supports employee schedule entries alongside meeting room reservations.

Supported work schedule types:

* external appointments
* work-from-home schedules
* leave schedules

Work schedules can be:

* created from the reservation screen
* displayed on the schedule screen
* shown together with room reservations
* opened in detail modals
* edited and deleted with permission checks

External appointments are treated as time-based schedules. Work-from-home and leave schedules are treated as day-level status entries, so they are visible in the main schedule list without overcrowding the timeline.

### Participant conflict warnings

The system detects participant-level conflicts, not only room conflicts.

Examples:

* a meeting room reservation overlaps with an external appointment for the same participant
* an external appointment overlaps with a meeting room reservation for the same participant
* two external appointments overlap for the same participant

These conflicts are shown as warnings rather than always blocking the operation, because real internal schedules sometimes intentionally overlap. The goal is to make conflicts visible before and after registration.

### In-app notifications

MeetingRoomBooker creates in-app notifications for affected users.

Notification examples:

* a participant was added to a reservation
* a participant was removed from a reservation
* a reservation was updated
* a work schedule was created
* a work schedule was updated
* a work schedule was deleted
* a participant schedule conflict exists

Notifications are associated with either a reservation or a work schedule entry through target IDs, making it easier to trace what each notification refers to.

### Chatwork notifications

Chatwork integration is handled entirely on the backend. The frontend never receives the Chatwork API token.

Supported Chatwork notification targets include:

* reservation organizers
* reservation participants
* users removed from a reservation
* work schedule participants
* users removed from a work schedule entry

Supported Chatwork notification events include:

* reservation creation
* reservation update
* reservation cancellation
* reservation reminders
* work schedule creation
* work schedule update
* work schedule deletion
* participant conflict warnings

Chatwork delivery is tracked per user, so one failed delivery does not block notifications for other users.

### Reminder delivery

A background worker checks for upcoming reservations and sends 10-minute reminder notifications through Chatwork.

Delivery is protected by delivery keys so the same reminder is not repeatedly sent to the same user.

### Admin and operations support

Admins can:

* register users
* update user names
* manage Chatwork account IDs
* manage Chatwork direct room IDs
* delete users
* manage reservations beyond normal ownership rules when necessary

An additional Operations Web UI is included for operational screens and admin-oriented workflows.

---

## Tech Stack

### Frontend

* Blazor WebAssembly
* Microsoft Fluent UI for Blazor
* Shared C# models
* React / TypeScript / Vite for Operations Web

### Backend

* C# / .NET 8
* ASP.NET Core Web API
* Entity Framework Core
* Cookie Authentication
* Hosted Services for background workers
* xUnit for automated tests

### Database and infrastructure

* SQLite
* EF Core migrations
* nginx
* systemd
* Linux VPS
* GitHub Actions

---

## Repository Structure

```text
MeetingRoomBooker.Api
  ASP.NET Core Web API
  Authentication, reservation APIs, work schedule APIs, notifications,
  Chatwork integration, background workers, and EF Core persistence

MeetingRoomBooker.Web
  Blazor WebAssembly frontend
  Reservation screen, schedule screen, notifications, settings, and user management UI

MeetingRoomBooker.Shared
  Shared DTOs and models used by both API and Web

MeetingRoomBooker.Tests
  xUnit tests for reservation rules, conflict detection, notifications,
  work schedule rules, and Chatwork notification behavior

MeetingRoomBooker.OperationsWeb
  React / TypeScript / Vite operations UI

docs
  Design notes, deployment notes, conflict detection documentation,
  and operational verification documents
```

---

## Architecture Overview

```text
+------------------------------+
| MeetingRoomBooker.Web        |
| Blazor WebAssembly           |
+--------------+---------------+
               |
               | HTTP API
               v
+--------------+---------------+
| MeetingRoomBooker.Api        |
| ASP.NET Core Web API         |
|                              |
| - Auth / Users               |
| - Reservations               |
| - Work schedules             |
| - Notifications              |
| - Chatwork delivery          |
| - Background workers         |
+--------------+---------------+
               |
               | EF Core
               v
+--------------+---------------+
| SQLite                       |
| app.db                       |
+------------------------------+
```

### Design principles

* Keep UI logic and business rules separate
* Enforce authorization and conflict validation on the API side
* Use shared models to reduce contract mismatches between frontend and backend
* Keep notification delivery on the backend
* Treat external integrations as operationally unreliable and log delivery results
* Keep production deployment simple enough for a small internal system
* Avoid casual production database migrations without backup and verification

---

## Domain Model Overview

### Reservations

A reservation represents a meeting room booking.

Main fields include:

* room
* date
* start time
* end time
* purpose
* reservation owner
* participants
* recurring series information

Room/time conflicts are validated before saving.

### Work schedule entries

A work schedule entry represents an employee schedule item.

Supported types:

* `ExternalAppointment`
* `WorkFromHome`
* `Leave`

External appointments have start and end times. Work-from-home and leave entries are day-level schedule entries.

### Notifications

Notifications can target either:

* a reservation
* a work schedule entry

This makes the notification model flexible enough to handle both meeting room workflows and employee schedule workflows.

### Chatwork delivery logs

Chatwork delivery logs track individual delivery attempts.

Each log can include:

* reservation ID
* work schedule entry ID
* delivery type
* delivery key
* target user ID
* room ID
* delivery status
* error message
* attempted time
* sent time
* message body

Delivery statuses include:

* `Succeeded`
* `Failed`
* `Skipped`

A skipped delivery usually means the target user does not have a direct Chatwork room ID configured.

---

## Conflict Handling

MeetingRoomBooker handles two types of conflicts.

### Room conflicts

Room conflicts are blocking conflicts.

If another reservation already uses the same room at an overlapping time, the API prevents the reservation from being saved unless the operation explicitly allows overlap in a controlled flow.

### Participant schedule conflicts

Participant conflicts are warning-based conflicts.

For example, a participant may already have an external appointment at the same time as a meeting room reservation.

These conflicts are not always blocked because some overlaps may be intentional, but they are surfaced through:

* confirmation dialogs in the UI
* warning icons on the schedule screen
* in-app warning notifications
* Chatwork warning messages

This design keeps the operation flexible while still making risky schedules visible.

---

## Notification Design

### In-app notifications

In-app notifications are created when users are affected by changes.

Examples:

* a user is added to a reservation
* a user is removed from a reservation
* a reservation is updated
* a work schedule is created
* a work schedule is updated
* a work schedule is deleted
* a participant conflict exists

Update notifications include change details where possible.

### Chatwork direct notifications

Chatwork direct notifications are sent to affected users.

For updates, the system separates users into:

* retained participants
* added participants
* removed participants

This allows the message to match the user's actual relationship to the change.

For example:

* retained users receive an update notification
* added users receive an "added" notification
* removed users receive a "removed" notification

### Duplicate delivery prevention

Each Chatwork notification uses a delivery key.

Example keys:

```text
ReservationCreated:reservation:{reservationId}:user:{targetUserId}
ReservationUpdated:reservation:{reservationId}:user:{targetUserId}:change:{changeId}
Reminder10Minutes:reservation:{reservationId}:user:{targetUserId}:start:{scheduledStartTime}

WorkScheduleCreated:work-schedule:{workScheduleEntryId}:user:{targetUserId}
WorkScheduleUpdated:work-schedule:{workScheduleEntryId}:user:{targetUserId}:change:{changeId}
WorkScheduleDeleted:work-schedule:{workScheduleEntryId}:user:{targetUserId}
```

This prevents duplicate delivery while still allowing each affected user to receive their own notification.

---

## Authentication and Authorization

### Authentication

* ASP.NET Core Cookie Authentication
* HttpOnly authentication cookies
* Remember Me support
* Persistent sessions for internal users

### Authorization

Regular users can:

* view schedules
* create reservations
* update or delete their own reservations
* join or leave reservations
* view their own notifications
* create and manage their own work schedule entries

Admins can:

* manage users
* manage Chatwork user mappings
* access admin-only APIs
* manage reservations and schedules beyond normal ownership rules when necessary

### Password handling

* New passwords are stored as hashes
* Legacy plaintext password values can be migrated after successful login
* Plaintext values are cleared after migration

---

## Testing

The project includes xUnit tests for important business rules.

Covered areas include:

* reservation access rules
* reservation overlap checks
* recurring reservation rules
* room conflict detection
* room conflict record management
* work schedule validation
* work schedule system notifications
* work schedule Chatwork notifications
* reservation Chatwork participant conflict warnings

Run all tests:

```bash
dotnet test MeetingRoomBooker.slnx
```

Build the solution:

```bash
dotnet build
```

---

## CI / Automation

GitHub Actions are used for build and verification.

Main workflows include:

* `.NET` build and test CI
* Operations Web build CI
* manual publish workflows
* manual VPS deployment workflows
* manual VPS deployment with migration confirmation

Production deployment workflows require explicit confirmation inputs so that schema-changing deployments are not triggered accidentally.

---

## Local Development

### Prerequisites

* .NET 8 SDK
* Visual Studio 2022 or VS Code
* Node.js for Operations Web development

### Run the API

```bash
cd MeetingRoomBooker.Api
dotnet restore
dotnet run
```

### Run the Blazor Web app

```bash
cd MeetingRoomBooker.Web
dotnet restore
dotnet run
```

### Run Operations Web

```bash
cd MeetingRoomBooker.OperationsWeb
npm install
npm run dev
```

### Build Operations Web

```bash
cd MeetingRoomBooker.OperationsWeb
npm run build
```

---

## Configuration

### Web configuration

`MeetingRoomBooker.Web/wwwroot/appsettings.json`

```json
{
  "ApiBaseUrl": "https://localhost:7005/"
}
```

### API configuration

`MeetingRoomBooker.Api/appsettings.json`

Important settings include:

* `ConnectionStrings:DefaultConnection`
* `Cors:AllowedOrigins`
* `Chatwork:Enabled`
* `Chatwork:ApiToken`
* `Chatwork:RoomId`
* `Chatwork:StakeholderRoomId`
* `Chatwork:ReceptionRoomId`
* `Chatwork:RoomMappings`
* `RoomConflictDetection`

Do not commit production secrets or real Chatwork tokens.

---

## Database and Migrations

The project uses SQLite with EF Core migrations.

Recent schema areas include:

* reservations
* users
* notifications
* Chatwork delivery logs
* room conflict records
* work schedule entries

Because this project can be used in a real internal environment, production database migrations must be handled carefully.

Before applying migrations to production:

1. confirm the actual production database path
2. back up the SQLite database with a timestamp
3. inspect `__EFMigrationsHistory`
4. inspect existing tables
5. test the migration against a copied database when possible
6. stop the API process
7. back up the database again
8. apply migrations
9. restart the API
10. verify core workflows
11. prepare rollback steps before starting

Do not run production migrations casually.

---

## Deployment Assumptions

This project is designed for a small internal, single-node deployment.

A typical production setup is:

```text
Browser
  |
  v
nginx
  |
  +--> static Blazor Web files
  |
  +--> ASP.NET Core API on localhost
          |
          v
        SQLite
```

Expected components:

* Ubuntu-based VPS
* nginx
* systemd service for the API
* static file hosting for the Blazor Web app
* SQLite database file
* manual deployment workflow with backup and rollback preparation

This keeps the system understandable and maintainable without introducing unnecessary infrastructure for a small team.

---

## Security and Privacy Notes

Before sharing or deploying this project, make sure to:

* remove all real credentials
* remove real Chatwork tokens
* avoid committing production database files
* avoid publishing screenshots with personal information
* avoid exposing VPS hostnames or private URLs
* verify that Git author email uses a safe public or noreply address

---

## What This Project Demonstrates

This project is intended to show practical application development skills beyond simple CRUD.

It demonstrates:

* real operational problem discovery
* backend-side business rule enforcement
* conflict detection design
* recurring reservation modeling
* user-aware notification design
* external API integration with delivery logging
* API-side authorization
* database migration awareness
* test coverage for service-level rules
* CI and deployment workflow organization
* production-minded rollback and backup thinking

---

## Possible Next Improvements

Areas I would improve next:

* add end-to-end tests for core reservation flows
* improve observability around notification delivery
* add richer audit logs for admin operations
* introduce stronger database-level guarantees for high-concurrency usage
* evaluate PostgreSQL migration if the system grows beyond small-team usage
* improve README screenshots and demo materials
* document production rollout steps in more detail

---

## Summary

MeetingRoomBooker is a meeting room booking and employee schedule management system designed around real internal operations.

It combines reservations, recurring schedules, work schedules, participant conflict warnings, notifications, Chatwork integration, tests, and production-aware deployment practices into one maintainable internal tool.

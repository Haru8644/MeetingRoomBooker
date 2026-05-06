# MeetingRoomBooker

MeetingRoomBooker is a meeting room booking system built with **Blazor WebAssembly** and **ASP.NET Core Web API** for small internal teams.

It was originally developed during my internship to improve day-to-day meeting room operations, including booking management, participant handling, notifications, and reminders.  
This public version has been cleaned up and reorganized so the architecture, authentication model, and deployment assumptions are easier to understand as a portfolio project, without exposing company-specific data or secrets.

---

## Overview

Managing meeting rooms through chat tools and shared calendars alone often creates operational friction:

- it is hard to see room availability at a glance
- ownership and participant responsibility can become unclear
- updates and cancellations are easy to miss
- reminder and reception workflows often depend on manual coordination
- overlapping reservations may be caught too late

MeetingRoomBooker brings these concerns into a single workflow that covers **reservations, access control, notifications, and day-to-day operational visibility**.

---

## Features

### Reservation management
- Create reservations with date, time, room, category, and purpose
- Add participants to a reservation
- View schedules in calendar and timeline formats
- Make organizers and participants easy to distinguish in the UI

### Recurring reservations
- Support daily and weekly recurring bookings
- Delete recurring reservations with scope options:
  - This occurrence only
  - This and following
  - Entire series
- Update recurring reservations with scope options:
  - Single occurrence
  - This and following
  - Entire series

### Notifications
- Notify users when participants are added or removed
- Send detailed update notifications based on reservation changes
- Notify organizers when someone joins or leaves a reservation
- Send direct Chatwork notifications to reservation owners and participants
- Send reminder notifications 10 minutes before the meeting starts
- Track Chatwork delivery results per target user for operational troubleshooting

### Operational support
- Cookie-based authentication with persistent sessions
- Remember Me support
- Admin user management
- Self-join flow for existing reservations
- Draft persistence while filling out reservation forms

### Security and access control
- Server-side authentication and authorization checks
- Reservation updates and deletions restricted to the organizer or an admin
- Notifications visible only to the relevant user or an admin
- Admin-only protection for management APIs
- Plaintext password storage removed for new registrations
- Legacy plaintext passwords can be migrated to hashed storage after a successful login

---

## Tech Stack

### Frontend
- Blazor WebAssembly
- Microsoft Fluent UI for Blazor
- Shared C# models and contracts

### Backend
- ASP.NET Core Web API
- Entity Framework Core
- Cookie Authentication
- Hosted Service for Chatwork reminders

### Data / Infrastructure
- SQLite
- Nginx
- Linux VPS
- systemd

---

## Architecture

```text
MeetingRoomBooker.Web     ... Blazor WebAssembly frontend
MeetingRoomBooker.Api     ... Authentication, reservation, notification, and Chatwork APIs
MeetingRoomBooker.Shared  ... Shared models and service contracts
```

### Design principles
- Keep frontend and backend responsibilities clearly separated
- Share contracts between client and server to reduce mismatches
- Enforce final authorization on the API side, not only in the UI
- Handle external notification delivery on the backend rather than exposing Chatwork directly from the frontend

---

## Authentication and Authorization

### Authentication
- ASP.NET Core Cookie Authentication
- Persistent sessions through Remember Me
- HttpOnly authentication cookies

### Authorization

Regular users can:
- view reservation schedules
- create reservations
- update or delete their own reservations
- join or leave other users' reservations
- view their own notifications

Admins can:
- list users
- register users
- update user names, Chatwork account IDs, and direct Chatwork room IDs
- delete users
- access admin-only management APIs
- manage reservations beyond normal ownership rules when necessary

### Password handling
- New passwords are stored as hashes
- Legacy plaintext passwords can be migrated after a successful login
- Plaintext values are cleared once migration is complete

---

## Reservation Integrity

The API performs **server-side overlap validation** before saving reservations, so duplicate bookings are not prevented by the UI alone.

This approach is suitable for a small internal deployment.  
In a higher-concurrency environment, I would strengthen this further with stricter transactional handling or database-level guarantees.

---

## Chatwork Integration

Chatwork integration is handled entirely on the backend.  
The frontend never receives the Chatwork API token.

### Supported events
- Reservation created
- Reservation updated
- Reminder sent 10 minutes before the meeting

### Direct notification targets
For direct Chatwork notifications, MeetingRoomBooker sends messages to:

- the reservation owner
- reservation participants
- users who were removed from a reservation after an update

This is designed so that users who are affected by a reservation change can still receive the update even if they are no longer part of the final participant list.

### User mapping
Each user can store two Chatwork-related identifiers:

- `ChatworkAccountId`: used for Chatwork mentions
- `ChatworkDirectRoomId`: used as the destination room ID for direct Chatwork messages

Admins can manage these values from the user management screen.

### Delivery logging
Chatwork delivery results are tracked per target user.

Each delivery log can include:

- `ReservationId`
- `DeliveryType`
- `DeliveryKey`
- `TargetUserId`
- `RoomId`
- `Status`
- `ErrorMessage`
- `AttemptedAt`
- `SentAt`

The delivery status can be:

- `Succeeded`: the message was sent successfully
- `Failed`: the system attempted to send the message, but Chatwork delivery failed
- `Skipped`: the target user did not have a direct Chatwork room ID configured

### Duplicate delivery prevention
Each direct notification uses a `DeliveryKey` to prevent duplicate delivery.

Examples:

```text
ReservationCreated:reservation:{reservationId}:user:{targetUserId}
ReservationUpdated:reservation:{reservationId}:user:{targetUserId}:change:{changeId}
Reminder10Minutes:reservation:{reservationId}:user:{targetUserId}:start:{scheduledStartTime}
```

This makes duplicate prevention user-aware.  
For example, the same reservation reminder can be sent once to each participant, while still preventing repeated delivery to the same user.

### Failure handling
Direct Chatwork delivery is processed user by user.

If delivery to one user fails, the system records a failed delivery log and continues processing the remaining users.  
This prevents a single Chatwork API failure or missing room setting from blocking notifications for everyone else.

### Reminder worker
The reminder worker is responsible for finding reservations that start soon.  
The actual direct delivery and delivery logging are delegated to the Chatwork notification service, keeping scheduling and delivery responsibilities separate.

---

## Local Development

### Prerequisites
- .NET 8 SDK
- Visual Studio 2022 or VS Code

### Run the API
```bash
cd MeetingRoomBooker.Api
dotnet restore
dotnet run
```

### Run the Web app
```bash
cd MeetingRoomBooker.Web
dotnet restore
dotnet run
```

### Create the first admin user
If the database is empty, the first registered user becomes the initial admin.

There is no public self-signup screen in the UI.  
For the first account only, create the user through:

- `POST /api/Users/register` in Swagger, or
- any API client

After that, additional users should be created from the admin management screen.

---

## Configuration

### Web
`MeetingRoomBooker.Web/wwwroot/appsettings.json`

```json
{
  "ApiBaseUrl": "https://localhost:7005/"
}
```

### API
`MeetingRoomBooker.Api/appsettings.json`

Main settings include:

- `ConnectionStrings:DefaultConnection`
- `Cors:AllowedOrigins`
- `Chatwork:Enabled`
- `Chatwork:ApiToken`
- `Chatwork:RoomId`
- `Chatwork:StakeholderRoomId`
- `Chatwork:ReceptionRoomId`
- `Chatwork:RoomMappings`

---

## Deployment Assumptions

This project is intended for a **small internal, single-node deployment**.

A typical setup looks like this:

- Web: static file hosting
- API: long-running Kestrel process
- Nginx: reverse proxy
- systemd: API process management
- SQLite: lightweight database for internal operational use

The goal is to keep the system understandable and maintainable without introducing unnecessary infrastructure complexity.

---

## What This Project Demonstrates

As a portfolio project, this repository is meant to highlight:

- practical internal tool design rather than toy CRUD
- API-side authorization instead of UI-only restrictions
- cookie-based authentication with persistent sessions
- recurring reservation workflows with scoped update and delete behavior
- operational notifications, reminder handling, and delivery logging
- backend-only Chatwork integration with user-level delivery tracking
- shared contracts between frontend and backend

---

## Possible Next Improvements

Areas I would improve next include:

- automated API tests and end-to-end tests
- clearer separation between controllers and application services
- stronger audit logging
- a smoother migration path from SQLite to PostgreSQL
- CI automation for build, test, format, and publish
- a more explicit database initialization and admin bootstrap flow

---

## Security and Privacy Notes

Before publishing or sharing a deployment based on this project, make sure to:

- remove all real email addresses, tokens, and secrets
- avoid committing local database files
- avoid publishing screenshots that contain personal information
- exclude production URLs, VPS details, and credentials

---

## Demo Materials

The following assets help make the project easier to present:

- login screen
- calendar screen
- reservation form
- admin management screen
- Chatwork notification examples

---

## Summary

> A meeting room booking system designed around real internal operational needs, covering reservations, access control, notifications, and reminders end to end.
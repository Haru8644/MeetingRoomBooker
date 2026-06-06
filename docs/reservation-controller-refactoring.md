# Reservation Controller Refactoring

## Overview

`ReservationsController` originally contained multiple responsibilities in one large controller, including request handling, reservation validation, overlap detection, recurring-series lookup, notification creation, and access checks.

This refactoring splits those responsibilities into focused services while preserving the existing reservation behavior.

The goal is to make the reservation feature easier to understand, test, and extend without changing the current API behavior.

## Background

MeetingRoomBooker is used to manage internal meeting room reservations.

The reservation feature includes several operational rules:

- Preventing or warning about overlapping reservations
- Managing recurring reservations
- Handling reservation participants
- Sending in-app notifications
- Sending Chatwork notifications
- Restricting update/delete operations to organizers and admins

As the feature grew, `ReservationsController` became responsible for too many concerns. This made the controller harder to review and increased the risk of accidental behavior changes.

## Refactoring Steps

### 1. ReservationRules

`ReservationRules` contains pure reservation rule logic that does not depend on HTTP, database access, or external APIs.

Responsibilities:

- Normalize reservation data
- Validate reservation date and time
- Detect recurring reservations
- Normalize series update/delete scope
- Clone reservations
- Apply reservation updates
- Build representative reservations for series notifications

This was extracted first because it has fewer side effects and is safer to test.

### 2. ReservationConflictService

`ReservationConflictService` contains overlap detection logic.

Responsibilities:

- Find conflicting reservations
- Exclude the current reservation when updating
- Build conflict response messages

Overlap prevention is one of the core values of MeetingRoomBooker, so moving this logic into a dedicated service makes the domain rule easier to find and maintain.

### 3. ReservationSeriesQueryService

`ReservationSeriesQueryService` contains recurring reservation series lookup logic.

Responsibilities:

- Find reservations that belong to the same series
- Support `single`, `following`, and `all` scopes
- Preserve fallback matching behavior for reservations without a `SeriesId`

The update/delete orchestration still remains outside this service. This service only handles the database query for finding related reservations.

### 4. ReservationNotificationService

`ReservationNotificationService` contains in-app reservation notification logic.

Responsibilities:

- Notify participants when they are added to a reservation
- Notify participants when a reservation is updated
- Notify participants when they are removed
- Notify the organizer when someone joins or leaves
- Create or update notification records

The controller still controls when `SaveChangesAsync` is called, so the existing persistence timing is preserved.

Chatwork notifications remain in the existing Chatwork notification service.

### 5. ReservationAccessService

`ReservationAccessService` contains reservation access-related logic.

Responsibilities:

- Read the current user ID from claims
- Load the current user from the database
- Check whether the current user can manage a reservation
- Allow organizers and admins to update/delete reservations

The controller still returns HTTP responses such as `Unauthorized`, `Forbid`, and `NotFound`, but the access logic itself is isolated.

## Current Controller Responsibility

After the refactoring, `ReservationsController` is still responsible for:

- Receiving HTTP requests
- Returning HTTP responses
- Calling application services
- Controlling database save timing
- Orchestrating reservation create/update/delete flows

The controller no longer directly owns the detailed rule logic for validation, conflict lookup, notification creation, recurring-series lookup, or access checks.

## Behavior Preservation

This refactoring is intended to preserve existing behavior.

The following points were intentionally kept unchanged:

- Reservation validation messages
- Overlap detection conditions
- Recurring reservation lookup behavior
- Notification messages
- Save timing around reservation and notification persistence
- Organizer/admin access rules
- Chatwork notification flow

## Verification

Each refactoring step was verified with:

- `dotnet build`
- `dotnet test`
- Manual checks for reservation create, update, delete, recurring reservation operations, overlap checks, and participant join/leave behavior

Additional tests were added after the refactoring to cover extracted reservation rules and access checks.

## Future Improvements

Possible next steps:

- Add more tests for `ReservationConflictService`
- Add more tests for `ReservationSeriesQueryService`
- Add integration tests for reservation create/update/delete APIs
- Consider extracting recurring update/delete orchestration into a dedicated service
- Keep Chatwork notification orchestration separate from in-app notification persistence

# Room Conflict Tracking Design

## Background

MeetingRoomBooker was built to improve internal meeting room reservation operations.

Before the system was introduced, meeting room reservations, outing schedules, and other internal schedules were managed together in TimeTree. This made it difficult to understand room availability, reservation ownership, and update responsibility. As a result, duplicate reservations and last-minute room conflicts occurred.

The current system supports reservation creation, calendar and timeline views, recurring reservations, participant management, authorization, server-side overlap checks, and Chatwork notifications.

However, the current system intentionally allows overlapping reservations after showing a warning. This is useful in some real operations because users may temporarily keep overlapping reservations and adjust them later.

Therefore, the next step is not simply to block all overlapping reservations. The system should record unresolved overlaps and actual room collisions so that the team can understand remaining operational issues and improve the process.

## Problem

A reservation data overlap and an actual room collision are not the same thing.

For example:

* Two reservations may overlap in the system, but one may be edited before the meeting starts.
* A user may see an overlap warning and adjust the reservation later.
* Two reservations may still overlap when the meeting start time arrives.
* A real room collision may happen because two groups arrive at the same room at the same time.
* A collision may happen because of a verbal reservation or a schedule outside the system.

If these situations are not recorded separately, it is difficult to know:

* how many overlaps remained unresolved,
* how many actual room collisions happened,
* what caused the collisions,
* how severe the impact was,
* and what should be improved next.

## Goal

The goal of this feature is to track room conflict events as operational feedback.

MeetingRoomBooker should not only help users create reservations, but also help the team observe real-world reservation failures, classify their causes, and improve meeting room operations over time.

This feature will support two main types of records:

1. unresolved reservation overlaps detected by the system,
2. actual room collisions reported by users or administrators.

## Non-goals

The first version will not automatically determine whether people physically arrived at the room at the same time.

The system can detect reservation overlaps from data, but actual room collisions depend on real-world events. Therefore, actual collisions should be confirmed or reported by humans.

The first version will also not build a full analytics dashboard. It will focus on recording, reviewing, and classifying conflict events.

## Core Concepts

### Reservation Overlap

A reservation overlap is a data-level overlap between two or more reservations.

Example:

* Reservation A: 10:00-11:00 / Main meeting room
* Reservation B: 10:30-11:30 / Main meeting room

This means the reservations overlap in the system.

The current system may show a warning and still allow the user to save the reservation.

### Unresolved Reservation Overlap

An unresolved reservation overlap is an overlap that still exists when the reservation start time arrives.

Example:

* Two reservations overlap in the same room.
* Neither reservation is edited or deleted.
* The reservation start time arrives while the overlap still exists.

In this case, the system should automatically create one conflict tracking record.

This record means that the overlap remained unresolved at the scheduled time. It does not necessarily mean that people physically collided at the room.

### Actual Room Collision

An actual room collision is a real-world event where multiple people or groups tried to use the same meeting room at the same time.

Example:

* Two groups arrived at the same room at 10:00.
* A meeting was delayed because another meeting was already using the room.
* A group had to move to another room at the last minute.
* A guest visit or business operation was affected.

Actual room collisions should be reported or confirmed by humans.

### Prevented Conflict

A prevented conflict is a potential conflict that was detected and resolved before it caused real-world impact.

Example:

* A user saw an overlap warning and changed the reservation time.
* An administrator noticed overlapping reservations and contacted the organizer before the meeting started.
* A reservation was deleted or moved before the overlap became unresolved.

This concept is valuable, but it will be treated as a future improvement.

## MVP Scope

The first version will include:

* automatic recording of unresolved reservation overlaps,
* manual reporting of actual room collisions,
* list view for conflict records,
* detail view for each record,
* edit form for classification and resolution notes,
* impact level,
* cause type,
* status,
* description,
* resolution,
* related reservation IDs when available.

The first version will not include:

* full analytics dashboard,
* automatic physical collision detection,
* CSV export,
* prevented conflict counting,
* Chatwork notification for conflict reports,
* advanced root-cause analysis.

## Conflict Record Types

The feature should distinguish between the following types:

```text
UnresolvedReservationOverlap
ActualRoomCollision
```

### UnresolvedReservationOverlap

Created automatically by the system when overlapping reservations remain unresolved at the scheduled start time.

### ActualRoomCollision

Created or confirmed manually when a real-world room collision happens.

## Conflict Status

Each record should have a status.

```text
Detected
Confirmed
FalseAlarm
Resolved
```

### Detected

The system detected an unresolved reservation overlap.

### Confirmed

A user or administrator confirmed that an actual room collision happened.

### FalseAlarm

The record was created from reservation data, but no actual collision happened.

### Resolved

The issue was handled and no further action is needed.

## Auto-detection Rule

The system should periodically check reservations and create a conflict record when all of the following conditions are met:

* the reservations are for the same meeting room,
* the reservations are on the same date,
* the reservation times overlap,
* both reservations still exist,
* the overlap still exists when the earliest reservation start time arrives,
* the same reservation pair has not already been recorded.

The system should create only one record for the same conflict pair.

## Duplicate Prevention for Auto Records

To avoid creating the same record repeatedly, the system should store a unique detection key.

The key may be based on:

* room name,
* date,
* reservation ID A,
* reservation ID B,
* overlap start time,
* overlap end time.

Example:

```text
MainRoom_2026-06-01_12_18_10:30_11:00
```

The exact format can be changed during implementation, but the purpose is to prevent duplicate conflict records from being created by the background worker.

## Data Model Draft

```csharp
public class RoomConflictRecord
{
    public int Id { get; set; }

    public ConflictRecordType Type { get; set; }
    public ConflictStatus Status { get; set; }

    public DateOnly OccurredDate { get; set; }
    public TimeOnly OccurredTime { get; set; }

    public string RoomName { get; set; } = string.Empty;

    public int? ReservationIdA { get; set; }
    public int? ReservationIdB { get; set; }

    public ConflictImpact Impact { get; set; }
    public ConflictCause Cause { get; set; }

    public string Description { get; set; } = string.Empty;
    public string Resolution { get; set; } = string.Empty;

    public string? DetectionKey { get; set; }

    public int? ReportedByUserId { get; set; }
    public DateTime CreatedAt { get; set; }
    public DateTime? UpdatedAt { get; set; }
}
```

Reservation IDs should be nullable because actual room collisions may involve schedules that were not registered in MeetingRoomBooker.

`DetectionKey` should be nullable because manually reported actual collisions may not have an automatic detection key.

## Enums Draft

```csharp
public enum ConflictRecordType
{
    UnresolvedReservationOverlap = 0,
    ActualRoomCollision = 1
}

public enum ConflictStatus
{
    Detected = 0,
    Confirmed = 1,
    FalseAlarm = 2,
    Resolved = 3
}

public enum ConflictImpact
{
    Low = 0,
    Medium = 1,
    High = 2
}

public enum ConflictCause
{
    ExistingReservationOverlooked = 0,
    ExternalCalendarConflict = 1,
    InputMistake = 2,
    NotificationMissed = 3,
    LastMinuteChange = 4,
    VerbalReservation = 5,
    Unknown = 98,
    Other = 99
}
```

## API Design Draft

The first version may provide the following endpoints:

```text
GET    /api/room-conflict-records
GET    /api/room-conflict-records/{id}
POST   /api/room-conflict-records
PUT    /api/room-conflict-records/{id}
GET    /api/room-conflict-records/summary
```

Delete will not be included in the MVP because conflict records are operational records.

If deletion is needed later, it should be limited to administrators or replaced with an archive feature.

## Background Worker Draft

A background worker may periodically check unresolved reservation overlaps.

Possible name:

```text
RoomConflictDetectionWorker
```

Responsibilities:

* load upcoming or recently started reservations,
* find overlapping reservations,
* check whether the overlap has reached the start time,
* generate a detection key,
* skip if the same key already exists,
* create a conflict record with type `UnresolvedReservationOverlap` and status `Detected`.

The worker should not send Chatwork notifications in the first version.

## Authorization Policy

The initial policy should be:

* Any logged-in user can create an actual room collision report.
* Any logged-in user can view conflict records.
* The report creator or an administrator can edit manually created records.
* Administrators can update all records.
* Auto-detected records can be classified by administrators.
* Deletion is not included in the MVP.

## UI Technology Decision

The backend will continue to use C# / ASP.NET Core Web API.

The existing reservation UI will remain in Blazor WebAssembly to avoid breaking the current production workflow.

The new conflict tracking UI will be implemented as part of a new React + TypeScript + Vite frontend.

Proposed project name:

```text
MeetingRoomBooker.OperationsWeb
```

This UI will be used for operational management features such as:

* room conflict tracking,
* employee status board,
* cleaning duty board,
* equipment reservation management.

This decision keeps the existing production system stable while introducing TypeScript for new, independent operational screens.

## UI Design Draft

The first UI should include:

* summary cards,
* conflict record list,
* filter by status,
* filter by impact,
* filter by room,
* detail view,
* manual report form,
* edit form for status, impact, cause, and resolution.

Summary cards may include:

* unresolved overlaps this month,
* confirmed actual collisions this month,
* high-impact conflicts,
* unresolved detected records.

## Repository Structure Draft

```text
MeetingRoomBooker/
  MeetingRoomBooker.Api/
  MeetingRoomBooker.Shared/
  MeetingRoomBooker.Web/
  MeetingRoomBooker.Tests/
  MeetingRoomBooker.OperationsWeb/
  docs/
```

React side draft:

```text
MeetingRoomBooker.OperationsWeb/
  src/
    app/
    features/
      conflictRecords/
        api/
        components/
        hooks/
        types/
      status/
      cleaning/
      equipment/
    shared/
      api/
      components/
      utils/
```

## Future Improvements

Future versions may add:

* prevented conflict counting,
* monthly conflict summary,
* cause-based aggregation,
* high-impact conflict notifications,
* Chatwork notification when a high-impact conflict is confirmed,
* CSV export,
* direct link from the Blazor schedule screen,
* OpenAPI-based TypeScript client generation,
* dashboard charts,
* false-alarm analysis.

## Evaluation Value

This feature changes MeetingRoomBooker from a reservation management tool into an operational improvement system.

It shows that the system does not only block or warn about duplicate reservations. It also observes unresolved overlaps, records actual room collisions, classifies their causes, and uses those records for continuous improvement.

This is valuable because real workplace operations cannot be improved only by adding forms. They need measurement, classification, feedback, and iteration.

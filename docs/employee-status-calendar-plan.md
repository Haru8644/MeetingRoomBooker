# Employee Status Calendar Plan

## Purpose

MeetingRoomBooker currently manages meeting room reservations. This feature extends the calendar so employees can also see external appointments, work-from-home schedules, and leave schedules in the same daily view.

The goal is to make the system closer to an internal operations platform, not just a meeting room booking tool.

## Scope

This feature adds the following schedule categories to the existing reservation calendar.

- 大会議室
- 小会議室
- 社外予定
- 在宅
- 休暇

## Design Policy

Work schedules are modeled separately from meeting room reservations.

External appointments, work-from-home schedules, and leave schedules are not room reservations, so they should not be stored in `ReservationModel` with fake room names.

Instead, they are stored as work schedule entries and merged with reservations only at the calendar display layer.

## Data Concepts

### WorkScheduleEntry

Represents scheduled employee availability information.

Supported types:

- ExternalAppointment
- WorkFromHome
- Leave

### LeavePeriod

Used only when the schedule type is Leave.

Supported values:

- None
- Morning
- Afternoon
- FullDay

## Production Rollout Notes

This feature will require a database schema change.

Do not deploy this feature to production using the normal deploy workflow immediately after merging.

Production rollout must use the migration workflow with the following steps:

1. Back up the production SQLite database.
2. Apply the migration.
3. Restart the API.
4. Check API logs.
5. Verify calendar behavior.
6. Keep rollback possible using the database backup.

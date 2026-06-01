namespace MeetingRoomBooker.Api.Models;

public sealed class RoomConflictRecord
{
    public int Id { get; set; }

    public ConflictRecordType Type { get; set; }
    public ConflictStatus Status { get; set; }

    public DateTime OccurredAt { get; set; }

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
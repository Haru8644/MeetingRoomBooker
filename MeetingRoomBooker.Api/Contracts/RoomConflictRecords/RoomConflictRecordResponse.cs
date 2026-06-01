using MeetingRoomBooker.Api.Models;

namespace MeetingRoomBooker.Api.Contracts.RoomConflictRecords;

public sealed class RoomConflictRecordResponse
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

    public bool CanEdit { get; set; }
}
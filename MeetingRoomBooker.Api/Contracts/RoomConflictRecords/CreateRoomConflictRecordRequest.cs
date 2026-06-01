using MeetingRoomBooker.Api.Models;

namespace MeetingRoomBooker.Api.Contracts.RoomConflictRecords;

public sealed class CreateRoomConflictRecordRequest
{
    public DateTime OccurredAt { get; set; }

    public string RoomName { get; set; } = string.Empty;

    public int? ReservationIdA { get; set; }

    public int? ReservationIdB { get; set; }

    public ConflictImpact Impact { get; set; } = ConflictImpact.Medium;

    public ConflictCause Cause { get; set; } = ConflictCause.Unknown;

    public string? Description { get; set; }

    public string? Resolution { get; set; }

    public ConflictStatus? Status { get; set; }
}
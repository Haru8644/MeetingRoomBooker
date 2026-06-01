using MeetingRoomBooker.Api.Models;

namespace MeetingRoomBooker.Api.Contracts.RoomConflictRecords;

public sealed class UpdateRoomConflictRecordRequest
{
    public ConflictStatus Status { get; set; }

    public ConflictImpact Impact { get; set; }

    public ConflictCause Cause { get; set; }

    public string? Description { get; set; }

    public string? Resolution { get; set; }
}
namespace MeetingRoomBooker.Api.Contracts.RoomConflictRecords;

public sealed class RoomConflictRecordSummaryResponse
{
    public int UnresolvedOverlapsThisMonth { get; set; }

    public int ConfirmedCollisionsThisMonth { get; set; }

    public int HighImpactConflictsThisMonth { get; set; }

    public int OpenDetectedRecords { get; set; }
}
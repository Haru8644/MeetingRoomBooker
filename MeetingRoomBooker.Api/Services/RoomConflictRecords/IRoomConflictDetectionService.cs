namespace MeetingRoomBooker.Api.Services.RoomConflictRecords;

public interface IRoomConflictDetectionService
{
    Task<int> DetectUnresolvedOverlapsAsync(
        DateTime now,
        TimeSpan lookbackWindow,
        CancellationToken cancellationToken);
}

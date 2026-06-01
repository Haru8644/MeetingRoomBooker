namespace MeetingRoomBooker.Api.Options;

public sealed class RoomConflictDetectionOptions
{
    public bool Enabled { get; set; }

    public int IntervalMinutes { get; set; } = 1;

    public int LookbackMinutes { get; set; } = 15;
}

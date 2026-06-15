namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public sealed record WorkScheduleParticipantConflict(
    int ParticipantUserId,
    string ParticipantName,
    string SourceType,
    int SourceId,
    string SourceTitle,
    DateTime Date,
    DateTime? StartTime,
    DateTime? EndTime);

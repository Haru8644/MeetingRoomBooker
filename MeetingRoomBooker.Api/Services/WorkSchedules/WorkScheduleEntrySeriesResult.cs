using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public sealed class WorkScheduleEntrySeriesResult
{
    private WorkScheduleEntrySeriesResult()
    {
    }

    public IReadOnlyList<WorkScheduleEntryModel> Entries { get; private init; } = Array.Empty<WorkScheduleEntryModel>();

    public string? ErrorMessage { get; private init; }

    public static WorkScheduleEntrySeriesResult Success(IReadOnlyList<WorkScheduleEntryModel> entries)
    {
        return new WorkScheduleEntrySeriesResult
        {
            Entries = entries
        };
    }

    public static WorkScheduleEntrySeriesResult BadRequest(string errorMessage)
    {
        return new WorkScheduleEntrySeriesResult
        {
            ErrorMessage = errorMessage
        };
    }
}

using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public sealed class WorkScheduleEntryResult
{
    private WorkScheduleEntryResult()
    {
    }

    public WorkScheduleEntryModel? Entry { get; private init; }

    public string? ErrorMessage { get; private init; }

    public bool NotFound { get; private init; }

    public bool Forbidden { get; private init; }

    public static WorkScheduleEntryResult Success(WorkScheduleEntryModel? entry = null)
    {
        return new WorkScheduleEntryResult
        {
            Entry = entry
        };
    }

    public static WorkScheduleEntryResult BadRequest(string errorMessage)
    {
        return new WorkScheduleEntryResult
        {
            ErrorMessage = errorMessage
        };
    }

    public static WorkScheduleEntryResult NotFoundResult()
    {
        return new WorkScheduleEntryResult
        {
            NotFound = true
        };
    }

    public static WorkScheduleEntryResult ForbiddenResult()
    {
        return new WorkScheduleEntryResult
        {
            Forbidden = true
        };
    }
}

using MeetingRoomBooker.Api.Contracts.RoomConflictRecords;

namespace MeetingRoomBooker.Api.Services.RoomConflictRecords;

public sealed class RoomConflictRecordResult
{
    private RoomConflictRecordResult()
    {
    }

    public RoomConflictRecordResponse? Record { get; private init; }

    public string? ErrorMessage { get; private init; }

    public bool NotFound { get; private init; }

    public bool Forbidden { get; private init; }

    public static RoomConflictRecordResult Success(RoomConflictRecordResponse record)
    {
        return new RoomConflictRecordResult
        {
            Record = record
        };
    }

    public static RoomConflictRecordResult BadRequest(string message)
    {
        return new RoomConflictRecordResult
        {
            ErrorMessage = message
        };
    }

    public static RoomConflictRecordResult NotFoundResult()
    {
        return new RoomConflictRecordResult
        {
            NotFound = true
        };
    }

    public static RoomConflictRecordResult ForbiddenResult()
    {
        return new RoomConflictRecordResult
        {
            Forbidden = true
        };
    }
}
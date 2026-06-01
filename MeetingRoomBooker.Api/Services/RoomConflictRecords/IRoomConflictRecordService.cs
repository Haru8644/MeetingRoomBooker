using MeetingRoomBooker.Api.Contracts.RoomConflictRecords;

namespace MeetingRoomBooker.Api.Services.RoomConflictRecords;

public interface IRoomConflictRecordService
{
    Task<IReadOnlyList<RoomConflictRecordResponse>> GetRecordsAsync(
        int currentUserId,
        bool isAdmin);

    Task<RoomConflictRecordResponse?> GetRecordAsync(
        int id,
        int currentUserId,
        bool isAdmin);

    Task<RoomConflictRecordResult> CreateManualRecordAsync(
        CreateRoomConflictRecordRequest request,
        int currentUserId,
        bool isAdmin);

    Task<RoomConflictRecordResult> UpdateRecordAsync(
        int id,
        UpdateRoomConflictRecordRequest request,
        int currentUserId,
        bool isAdmin);

    Task<RoomConflictRecordSummaryResponse> GetSummaryAsync(DateTime now);
}
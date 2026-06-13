using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public interface IWorkScheduleEntryService
{
    Task<IReadOnlyList<WorkScheduleEntryModel>> GetEntriesAsync(
        DateTime? from,
        DateTime? to,
        CancellationToken cancellationToken);

    Task<WorkScheduleEntryModel?> GetEntryAsync(
        int id,
        CancellationToken cancellationToken);

    Task<WorkScheduleEntryResult> CreateEntryAsync(
        CreateWorkScheduleEntryRequest request,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken);

    Task<WorkScheduleEntryResult> UpdateEntryAsync(
        int id,
        UpdateWorkScheduleEntryRequest request,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken);

    Task<WorkScheduleEntryResult> DeleteEntryAsync(
        int id,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken);
}

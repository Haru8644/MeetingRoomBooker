using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public interface IWorkScheduleNotificationService
{
    Task NotifyCreatedAsync(
        WorkScheduleEntryModel entry,
        CancellationToken cancellationToken);

    Task NotifySeriesCreatedAsync(
        IReadOnlyList<WorkScheduleEntryModel> entries,
        CancellationToken cancellationToken);

    Task NotifyUpdatedAsync(
        WorkScheduleEntryModel previousEntry,
        WorkScheduleEntryModel currentEntry,
        CancellationToken cancellationToken);

    Task NotifyDeletedAsync(
        WorkScheduleEntryModel entry,
        CancellationToken cancellationToken);

    Task NotifySeriesDeletedAsync(
        IReadOnlyList<WorkScheduleEntryModel> entries,
        string scope,
        CancellationToken cancellationToken);
}

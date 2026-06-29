using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IWorkScheduleChatworkNotificationService
    {
        Task SendCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default);

        Task SendSeriesCreatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            CancellationToken cancellationToken = default);

        Task SendUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken = default);

        Task SendSeriesUpdatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> previousEntries,
            IReadOnlyList<WorkScheduleEntryModel> currentEntries,
            string scope,
            CancellationToken cancellationToken = default);

        Task SendDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default);

        Task SendReminderAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default);

        Task SendSeriesDeletedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            string scope,
            CancellationToken cancellationToken = default);
    }
}

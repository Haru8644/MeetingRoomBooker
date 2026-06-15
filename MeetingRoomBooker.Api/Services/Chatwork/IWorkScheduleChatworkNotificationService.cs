using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IWorkScheduleChatworkNotificationService
    {
        Task SendCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default);

        Task SendUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken = default);

        Task SendDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default);
    }
}

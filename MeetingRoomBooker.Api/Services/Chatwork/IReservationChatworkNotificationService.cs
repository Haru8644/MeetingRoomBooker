using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IReservationChatworkNotificationService
    {
        Task SendReservationCreatedAsync(ReservationModel reservation, CancellationToken cancellationToken = default);
        Task SendReservationUpdatedAsync(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            CancellationToken cancellationToken = default);
        Task SendReservationCanceledAsync(ReservationModel reservation, CancellationToken cancellationToken = default);
        Task SendReservationReminderAsync(ReservationModel reservation, CancellationToken cancellationToken = default);
    }
}
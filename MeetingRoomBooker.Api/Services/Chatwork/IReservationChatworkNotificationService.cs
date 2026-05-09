using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IReservationChatworkNotificationService
    {
        Task SendReservationCreatedAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default);

        Task SendReservationCreatedAsync(
            ReservationModel reservation,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default);

        Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            CancellationToken cancellationToken = default);

        Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default);

        Task SendReservationUpdatedAsync(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            CancellationToken cancellationToken = default);

        Task SendReservationSeriesUpdatedAsync(
            ReservationModel representativePreviousReservation,
            ReservationModel representativeCurrentReservation,
            int updatedCount,
            CancellationToken cancellationToken = default);

        Task SendReservationCanceledAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default);

        Task SendReservationSeriesCanceledAsync(
            ReservationModel representativeReservation,
            int canceledCount,
            CancellationToken cancellationToken = default);

        Task SendReservationReminderAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default);
    }
}

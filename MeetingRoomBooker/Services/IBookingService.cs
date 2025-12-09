using MeetingRoomBooker.Models;

namespace MeetingRoomBooker.Services
{
    public interface IBookingService
    {
        Task<List<ReservationModel>> GetReservationsAsync();
        Task AddReservationAsync(ReservationModel reservation);
        Task RemoveReservationAsync(ReservationModel reservation);
        Task UpdateReservationAsync(ReservationModel reservation);
    }
}
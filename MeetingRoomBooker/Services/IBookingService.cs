using MeetingRoomBooker.Models;

namespace MeetingRoomBooker.Services
{
    public interface IBookingService
    {
        Task<List<ReservationModel>> GetReservationsAsync();
        Task AddReservationAsync(ReservationModel reservation);
        Task RemoveReservationAsync(ReservationModel reservation);
        Task UpdateReservationAsync(ReservationModel reservation);
        Task<UserModel?> LoginAsync(string email, string password);
        Task RegisterUserAsync(UserModel user);
        Task<List<UserModel>> GetAllUsersAsync();
        UserModel? GetCurrentUser(); 
        void Logout();
        Task<List<NotificationModel>> GetNotificationsAsync(int userId);
        Task AddNotificationAsync(NotificationModel notification);
        Task MarkNotificationAsReadAsync(int notificationId);
        event Action OnChange;
    }
}
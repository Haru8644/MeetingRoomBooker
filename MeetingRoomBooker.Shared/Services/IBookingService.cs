using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Shared.Services
{
    public interface IBookingService
    {
        Task InitializeAsync() => Task.CompletedTask;
        Task<UserModel?> LoginAsync(string email, string password);
        Task<UserModel?> LoginAsync(string email, string password, bool rememberMe) => LoginAsync(email, password);
        Task<bool> RegisterUserAsync(UserModel user);
        Task DeleteUserAsync(int userId);
        Task UpdateUserNameAsync(int userId, string name);
        Task UpdateUserChatworkAccountIdAsync(int userId, string? chatworkAccountId);
        void Logout();
        Task LogoutAsync()
        {
            Logout();
            return Task.CompletedTask;
        }
        UserModel? GetCurrentUser();
        Task<List<ReservationModel>> GetReservationsAsync();
        Task AddReservationAsync(ReservationModel reservation);
        Task RemoveReservationAsync(ReservationModel reservation);
        Task RemoveRecurringReservationAsync(ReservationModel reservation, string scope);
        Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify);
        Task UpdateRecurringReservationAsync(ReservationModel originalReservation, ReservationModel updatedReservation, bool shouldNotify, string scope);
        Task JoinReservationAsync(int reservationId);
        Task LeaveReservationAsync(int reservationId);
        Task<List<UserModel>> GetAllUsersAsync();
        Task<List<NotificationModel>> GetNotificationsAsync(int userId);
        Task AddNotificationAsync(NotificationModel notification);
        Task DeleteNotificationAsync(int notificationId);
        Task MarkNotificationAsReadAsync(int notificationId);
        event Action? OnChange;
    }
}
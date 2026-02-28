using MeetingRoomBooker.Shared.Models;
<<<<<<< HEAD:MeetingRoomBooker.Shared/Services/IBookingService.cs
using System.Collections.Generic;
using System.Threading.Tasks;
=======
>>>>>>> origin/master:MeetingRoomBooker/Services/IBookingService.cs

namespace MeetingRoomBooker.Services
{
    public interface IBookingService
    {
        Task<UserModel> LoginAsync(string email, string password);
        Task<bool> RegisterUserAsync(UserModel user);
        Task DeleteUserAsync(int userId);
        void Logout();
        UserModel? GetCurrentUser();
        Task<List<ReservationModel>> GetReservationsAsync();
        Task AddReservationAsync(ReservationModel reservation);
        Task RemoveReservationAsync(ReservationModel reservation);
        Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify);
        Task DeleteNotificationAsync(int notificationId);
        Task<List<UserModel>> GetAllUsersAsync();
        Task<List<NotificationModel>> GetNotificationsAsync(int userId);
        Task AddNotificationAsync(NotificationModel notification);
        Task MarkNotificationAsReadAsync(int notificationId);
        event System.Action? OnChange;
    }
}
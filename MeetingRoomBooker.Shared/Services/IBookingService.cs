using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Shared.Services
{
    public interface IBookingService
    {
        Task<UserModel?> LoginAsync(string email, string password);
        Task<bool> RegisterUserAsync(UserModel user);
        Task DeleteUserAsync(int userId);

        void Logout();
        UserModel? GetCurrentUser();

        Task<List<ReservationModel>> GetReservationsAsync();
        Task AddReservationAsync(ReservationModel reservation);
        Task RemoveReservationAsync(ReservationModel reservation);
        Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify);

        Task<List<UserModel>> GetAllUsersAsync();

        Task<List<NotificationModel>> GetNotificationsAsync(int userId);
        Task AddNotificationAsync(NotificationModel notification);
        Task DeleteNotificationAsync(int notificationId);
        Task MarkNotificationAsReadAsync(int notificationId);

        event Action? OnChange;
    }
}
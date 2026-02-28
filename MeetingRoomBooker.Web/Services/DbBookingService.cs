using MeetingRoomBooker.Shared.Models;
using System;
using System.Collections.Generic;
using System.Threading.Tasks;

namespace MeetingRoomBooker.Services
{
    public class DbBookingService : IBookingService
    {
        public event Action? OnChange;
        public Task DeleteUserAsync(int userId) { return Task.CompletedTask; }
        public Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify) { return Task.CompletedTask; }
        public Task DeleteNotificationAsync(int notificationId) => Task.CompletedTask;
        public Task<UserModel> LoginAsync(string email, string password) => Task.FromResult<UserModel>(null);
        public Task<bool> RegisterUserAsync(UserModel user) => Task.FromResult(false);
        public void Logout() { }
        public UserModel? GetCurrentUser() => null;
        public Task<List<ReservationModel>> GetReservationsAsync() => Task.FromResult(new List<ReservationModel>());
        public Task AddReservationAsync(ReservationModel reservation) => Task.CompletedTask;
        public Task RemoveReservationAsync(ReservationModel reservation) => Task.CompletedTask;
        public Task<List<UserModel>> GetAllUsersAsync() => Task.FromResult(new List<UserModel>());
        public Task<List<NotificationModel>> GetNotificationsAsync(int userId) => Task.FromResult(new List<NotificationModel>());
        public Task AddNotificationAsync(NotificationModel notification) => Task.CompletedTask;
        public Task MarkNotificationAsReadAsync(int notificationId) => Task.CompletedTask;
    }
}
using MeetingRoomBooker.Shared.Models;
using System.Net.Http.Json;
using System.Linq;

namespace MeetingRoomBooker.Services
{
    public class ApiBookingService : IBookingService
    {
        private readonly HttpClient _http;
        public event Action? OnChange;
        public UserModel? CurrentUser { get; private set; }

        public ApiBookingService(HttpClient http) { _http = http; }

        public UserModel? GetCurrentUser() => CurrentUser;

        public async Task<UserModel> LoginAsync(string email, string password)
        {
            var response = await _http.PostAsJsonAsync("api/users/login", new { Email = email, Password = password });
            if (response.IsSuccessStatusCode)
            {
                CurrentUser = await response.Content.ReadFromJsonAsync<UserModel>();
                NotifyStateChanged();
                return CurrentUser;
            }
            return null;
        }

        public void Logout() { CurrentUser = null; NotifyStateChanged(); }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            return await _http.GetFromJsonAsync<List<ReservationModel>>("api/reservations") ?? new List<ReservationModel>();
        }

        public async Task AddReservationAsync(ReservationModel reservation)
        {
            await _http.PostAsJsonAsync("api/reservations", reservation);
            NotifyStateChanged();
        }

        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            await _http.DeleteAsync($"api/reservations/{reservation.Id}");
            NotifyStateChanged();
        }

        public async Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify)
        {
            var all = await GetReservationsAsync();
            var original = all.FirstOrDefault(x => x.Id == reservation.Id);

            if (original != null && shouldNotify)
            {
                var oldIds = original.ParticipantIds ?? new List<int>();
                var newIds = reservation.ParticipantIds ?? new List<int>();
                var addedIds = newIds.Except(oldIds).ToList();
                foreach (var pid in addedIds)
                {
                    if (CurrentUser != null && pid == CurrentUser.Id) continue;
                    var notif = new NotificationModel { UserId = pid, Type = "Info", Message = $"予約「{reservation.Purpose}」の参加者に追加されました。", TargetDate = reservation.Date, TargetReservationId = reservation.Id, IsRead = false };
                    await AddNotificationAsync(notif);
                }

                var removedIds = oldIds.Except(newIds).ToList();
                foreach (var pid in removedIds)
                {
                    if (CurrentUser != null && pid == CurrentUser.Id) continue;
                    var notif = new NotificationModel { UserId = pid, Type = "Warning", Message = $"予約「{reservation.Purpose}」の参加者から外されました。", TargetDate = reservation.Date, IsRead = false };
                    await AddNotificationAsync(notif);
                }

                var remainingIds = newIds.Intersect(oldIds).ToList();
                foreach (var pid in remainingIds)
                {
                    if (CurrentUser != null && pid == CurrentUser.Id) continue;
                    var notif = new NotificationModel { UserId = pid, Type = "Info", Message = $"参加中の予約「{reservation.Purpose}」の内容が変更されました。", TargetDate = reservation.Date, TargetReservationId = reservation.Id, IsRead = false };
                    await AddNotificationAsync(notif);
                }
            }

            await _http.PutAsJsonAsync($"api/reservations/{reservation.Id}", reservation);
            NotifyStateChanged();
        }

        public async Task DeleteNotificationAsync(int notificationId)
        {
            await _http.DeleteAsync($"api/notifications/{notificationId}");
            NotifyStateChanged();
        }

        public async Task<List<UserModel>> GetAllUsersAsync()
        {
            return await _http.GetFromJsonAsync<List<UserModel>>("api/users") ?? new List<UserModel>();
        }

        public async Task<bool> RegisterUserAsync(UserModel user)
        {
            var response = await _http.PostAsJsonAsync("api/users/register", user);
            return response.IsSuccessStatusCode;
        }

        public async Task DeleteUserAsync(int userId)
        {
            await _http.DeleteAsync($"api/users/{userId}");
            NotifyStateChanged();
        }

        public async Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            return await _http.GetFromJsonAsync<List<NotificationModel>>($"api/notifications/user/{userId}") ?? new List<NotificationModel>();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            await _http.PutAsync($"api/notifications/{notificationId}/read", null);
            NotifyStateChanged();
        }

        public async Task AddNotificationAsync(NotificationModel notification)
        {
            await _http.PostAsJsonAsync("api/notifications", notification);
        }

        private void NotifyStateChanged() => OnChange?.Invoke();
    }
}
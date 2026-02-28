using System.Net.Http.Json;
using System.Text.Json;
using System.Diagnostics;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Services
{
    public class ApiBookingService : IBookingService
    {
        private readonly HttpClient _httpClient;
        private UserModel? _currentUser;
        public event Action? OnChange;
        private void NotifyStateChanged() => OnChange?.Invoke();

        private readonly JsonSerializerOptions _options = new JsonSerializerOptions
        {
            PropertyNameCaseInsensitive = true
        };

        public ApiBookingService(HttpClient httpClient)
        {
            _httpClient = httpClient;
        }

        public async Task<UserModel?> LoginAsync(string email, string password)
        {
            Debug.WriteLine($"[Login] ログイン開始: Email={email}");

            try
            {
                var loginData = new { Email = email, Password = password, Name = "Guest" };

                var response = await _httpClient.PostAsJsonAsync("api/Users/login", loginData);

                if (response.IsSuccessStatusCode)
                {
                    _currentUser = await response.Content.ReadFromJsonAsync<UserModel>(_options);
                    Debug.WriteLine($"[Login] 成功！ ユーザー名: {_currentUser?.Name}");
                    NotifyStateChanged();
                    return _currentUser;
                }
                else
                {
                    var error = await response.Content.ReadAsStringAsync();
                    Debug.WriteLine($"[Login] 失敗 (ステータス: {response.StatusCode})");
                    Debug.WriteLine($"[Login] サーバーのエラー文: {error}");
                    return null;
                }
            }
            catch (Exception ex)
            {
                Debug.WriteLine($"[Login] 通信エラー発生: {ex.Message}");
                return null;
            }
        }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            return await _httpClient.GetFromJsonAsync<List<ReservationModel>>("api/Reservations", _options) ?? new List<ReservationModel>();
        }

        public async Task AddReservationAsync(ReservationModel reservation)
        {
            if (_currentUser != null) reservation.UserId = _currentUser.Id;
            else reservation.UserId = 1; 

            await _httpClient.PostAsJsonAsync("api/Reservations", reservation);
            NotifyStateChanged();
        }

        public async Task UpdateReservationAsync(ReservationModel reservation)
        {
            await _httpClient.PutAsJsonAsync($"api/Reservations/{reservation.Id}", reservation);
            NotifyStateChanged();
        }

        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            await _httpClient.DeleteAsync($"api/Reservations/{reservation.Id}");
            NotifyStateChanged();
        }

        public async Task RegisterUserAsync(UserModel user)
        {
            var response = await _httpClient.PostAsJsonAsync("api/Users/register", user);
            if (response.IsSuccessStatusCode)
            {
                _currentUser = await response.Content.ReadFromJsonAsync<UserModel>(_options);
                NotifyStateChanged();
            }
            else
            {
                throw new Exception("Registration failed");
            }
        }

        public async Task<List<UserModel>> GetAllUsersAsync()
        {
            return await _httpClient.GetFromJsonAsync<List<UserModel>>("api/Users", _options) ?? new List<UserModel>();
        }

        public UserModel? GetCurrentUser() => _currentUser;

        public void Logout()
        {
            _currentUser = null;
            NotifyStateChanged();
        }

        public async Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            return await _httpClient.GetFromJsonAsync<List<NotificationModel>>($"api/Notifications/user/{userId}", _options) ?? new List<NotificationModel>();
        }

        public async Task AddNotificationAsync(NotificationModel notification)
        {
            await _httpClient.PostAsJsonAsync("api/Notifications", notification);
            NotifyStateChanged();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            await _httpClient.PutAsync($"api/Notifications/{notificationId}/read", null);
            NotifyStateChanged();
        }
    }
}
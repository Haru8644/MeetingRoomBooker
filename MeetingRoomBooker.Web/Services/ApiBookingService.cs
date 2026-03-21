using System.Diagnostics;
using System.Net.Http.Json;
using System.Text.Json;
using MeetingRoomBooker.Shared.Models;
using MeetingRoomBooker.Shared.Services;

namespace MeetingRoomBooker.Web.Services
{
    public sealed class ApiBookingService : IBookingService
    {
        private readonly HttpClient _httpClient;
        private UserModel? _currentUser;

        public event Action? OnChange;

        private void NotifyStateChanged() => OnChange?.Invoke();

        private static readonly JsonSerializerOptions JsonOptions = new()
        {
            PropertyNameCaseInsensitive = true
        };

        public ApiBookingService(HttpClient httpClient)
        {
            _httpClient = httpClient;
        }

        public async Task<UserModel?> LoginAsync(string email, string password)
        {
            try
            {
                var loginData = new { Email = email, Password = password };
                var response = await _httpClient.PostAsJsonAsync("api/Users/login", loginData);

                if (!response.IsSuccessStatusCode)
                {
                    Debug.WriteLine($"[Login] Failed: {response.StatusCode}");
                    return null;
                }

                _currentUser = await response.Content.ReadFromJsonAsync<UserModel>(JsonOptions);
                NotifyStateChanged();
                return _currentUser;
            }
            catch (Exception ex)
            {
                Debug.WriteLine($"[Login] Error: {ex.Message}");
                return null;
            }
        }

        public async Task<bool> RegisterUserAsync(UserModel user)
        {
            var requestUrl = new Uri(_httpClient.BaseAddress!, "api/Users/register");

            var requestJson = JsonSerializer.Serialize(user, new JsonSerializerOptions
            {
                WriteIndented = true
            });

            Console.WriteLine("===== REGISTER REQUEST START =====");
            Console.WriteLine($"POST {requestUrl}");
            Console.WriteLine(requestJson);
            Console.WriteLine("===== REGISTER REQUEST END =====");

            try
            {
                var response = await _httpClient.PostAsJsonAsync("api/Users/register", user);
                var responseBody = await response.Content.ReadAsStringAsync();

                Console.WriteLine("===== REGISTER RESPONSE START =====");
                Console.WriteLine($"Status: {(int)response.StatusCode} {response.StatusCode}");
                Console.WriteLine(responseBody);
                Console.WriteLine("===== REGISTER RESPONSE END =====");

                if (!response.IsSuccessStatusCode)
                {
                    throw new InvalidOperationException(
                        $"Register failed. Status={(int)response.StatusCode} {response.StatusCode}. Body={responseBody}");
                }

                NotifyStateChanged();
                return true;
            }
            catch (Exception ex)
            {
                Console.WriteLine("===== REGISTER EXCEPTION START =====");
                Console.WriteLine(ex.ToString());
                Console.WriteLine("===== REGISTER EXCEPTION END =====");
                throw;
            }
        }
        public async Task DeleteUserAsync(int userId)
        {
            var response = await _httpClient.DeleteAsync($"api/Users/{userId}");
            await EnsureSuccessAsync(response, $"User deletion failed for id={userId}.");

            if (_currentUser?.Id == userId)
            {
                _currentUser = null;
                NotifyStateChanged();
            }
        }

        public async Task UpdateUserChatworkAccountIdAsync(int userId, string? chatworkAccountId)
        {
            var response = await _httpClient.PutAsJsonAsync(
                $"api/Users/{userId}/chatwork-account",
                new { ChatworkAccountId = chatworkAccountId });

            await EnsureSuccessAsync(response, $"Failed to update Chatwork account id for user id={userId}.");

            if (_currentUser?.Id == userId)
            {
                _currentUser.ChatworkAccountId = string.IsNullOrWhiteSpace(chatworkAccountId)
                    ? null
                    : chatworkAccountId.Trim();
            }

            NotifyStateChanged();
        }

        public void Logout()
        {
            _currentUser = null;
            NotifyStateChanged();
        }

        public UserModel? GetCurrentUser() => _currentUser;

        public async Task<List<UserModel>> GetAllUsersAsync()
        {
            return await _httpClient.GetFromJsonAsync<List<UserModel>>("api/Users", JsonOptions)
                   ?? new List<UserModel>();
        }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            return await _httpClient.GetFromJsonAsync<List<ReservationModel>>("api/Reservations", JsonOptions)
                   ?? new List<ReservationModel>();
        }

        public async Task AddReservationAsync(ReservationModel reservation)
        {
            if (_currentUser != null && reservation.UserId == 0)
            {
                reservation.UserId = _currentUser.Id;
            }

            var response = await _httpClient.PostAsJsonAsync("api/Reservations", reservation);
            await EnsureSuccessAsync(response, "Failed to add reservation.");
            NotifyStateChanged();
        }

        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            var response = await _httpClient.DeleteAsync($"api/Reservations/{reservation.Id}");
            await EnsureSuccessAsync(response, $"Failed to remove reservation id={reservation.Id}.");
            NotifyStateChanged();
        }

        public async Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify)
        {
            var response = await _httpClient.PutAsJsonAsync(
                $"api/Reservations/{reservation.Id}?notifyParticipants={shouldNotify.ToString().ToLowerInvariant()}",
                reservation);

            await EnsureSuccessAsync(response, $"Failed to update reservation id={reservation.Id}.");
            NotifyStateChanged();
        }

        public async Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            return await _httpClient.GetFromJsonAsync<List<NotificationModel>>(
                       $"api/Notifications/user/{userId}", JsonOptions)
                   ?? new List<NotificationModel>();
        }

        public async Task AddNotificationAsync(NotificationModel notification)
        {
            var response = await _httpClient.PostAsJsonAsync("api/Notifications", notification);
            await EnsureSuccessAsync(response, "Failed to add notification.");
            NotifyStateChanged();
        }

        public async Task DeleteNotificationAsync(int notificationId)
        {
            var response = await _httpClient.DeleteAsync($"api/Notifications/{notificationId}");
            await EnsureSuccessAsync(response, $"Failed to delete notification id={notificationId}.");
            NotifyStateChanged();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            var response = await _httpClient.PutAsync($"api/Notifications/{notificationId}/read", null);
            await EnsureSuccessAsync(response, $"Failed to mark notification id={notificationId} as read.");
            NotifyStateChanged();
        }

        private static async Task EnsureSuccessAsync(HttpResponseMessage response, string fallbackMessage)
        {
            if (response.IsSuccessStatusCode)
            {
                return;
            }

            var details = await response.Content.ReadAsStringAsync();
            throw new InvalidOperationException(string.IsNullOrWhiteSpace(details)
                ? fallbackMessage
                : details);
        }
    }
}
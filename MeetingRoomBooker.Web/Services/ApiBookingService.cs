using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using MeetingRoomBooker.Shared.Models;
using MeetingRoomBooker.Shared.Services;
using Microsoft.AspNetCore.Components.WebAssembly.Http;

namespace MeetingRoomBooker.Web.Services
{
    public sealed class ApiBookingService : IBookingService
    {
        private readonly HttpClient _httpClient;
        private UserModel? _currentUser;
        private bool _isInitialized;

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

        public async Task InitializeAsync()
        {
            if (_isInitialized)
            {
                return;
            }

            _currentUser = await GetSessionUserAsync();
            _isInitialized = true;
            NotifyStateChanged();
        }

        public Task<UserModel?> LoginAsync(string email, string password)
        {
            return LoginAsync(email, password, false);
        }

        public async Task<UserModel?> LoginAsync(string email, string password, bool rememberMe)
        {
            var response = await SendAsync(
                HttpMethod.Post,
                "api/Auth/login",
                new LoginRequest
                {
                    Email = email,
                    Password = password,
                    RememberMe = rememberMe
                });

            if (!response.IsSuccessStatusCode)
            {
                return null;
            }

            _currentUser = await ReadFromJsonAsync<UserModel>(response);
            _isInitialized = true;
            NotifyStateChanged();
            return _currentUser;
        }

        public async Task<bool> RegisterUserAsync(UserModel user)
        {
            var response = await SendAsync(
                HttpMethod.Post,
                "api/Users/register",
                new RegisterUserRequest
                {
                    Name = user.Name?.Trim() ?? string.Empty,
                    Email = user.Email?.Trim() ?? string.Empty,
                    Password = user.Password ?? string.Empty,
                    AvatarColor = string.IsNullOrWhiteSpace(user.AvatarColor) ? "#a371f7" : user.AvatarColor,
                    ChatworkAccountId = string.IsNullOrWhiteSpace(user.ChatworkAccountId)
                        ? null
                        : user.ChatworkAccountId.Trim()
                });

            if (!response.IsSuccessStatusCode)
            {
                return false;
            }

            NotifyStateChanged();
            return true;
        }

        public async Task DeleteUserAsync(int userId)
        {
            var response = await SendAsync(HttpMethod.Delete, $"api/Users/{userId}");
            await EnsureSuccessAsync(response, $"User deletion failed for id={userId}.");

            if (_currentUser?.Id == userId)
            {
                _currentUser = null;
                NotifyStateChanged();
            }
        }

        public async Task UpdateUserNameAsync(int userId, string name)
        {
            var normalizedName = name?.Trim() ?? string.Empty;

            var response = await SendAsync(
                HttpMethod.Put,
                $"api/Users/{userId}/name",
                new { Name = normalizedName });

            await EnsureSuccessAsync(response, $"Failed to update user name for user id={userId}.");

            if (_currentUser?.Id == userId)
            {
                _currentUser.Name = normalizedName;
            }

            NotifyStateChanged();
        }

        public async Task UpdateUserChatworkAccountIdAsync(int userId, string? chatworkAccountId)
        {
            var response = await SendAsync(
                HttpMethod.Put,
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
            _ = LogoutAsync();
        }

        public async Task LogoutAsync()
        {
            try
            {
                var response = await SendAsync(HttpMethod.Post, "api/Auth/logout");

                if (response.StatusCode != HttpStatusCode.Unauthorized)
                {
                    await EnsureSuccessAsync(response, "Failed to log out.");
                }
            }
            finally
            {
                _currentUser = null;
                _isInitialized = true;
                NotifyStateChanged();
            }
        }

        public UserModel? GetCurrentUser() => _currentUser;

        public async Task<List<UserModel>> GetAllUsersAsync()
        {
            var response = await SendAsync(HttpMethod.Get, "api/Users");
            await EnsureSuccessAsync(response, "Failed to load users.");
            return await ReadFromJsonAsync<List<UserModel>>(response) ?? new List<UserModel>();
        }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            var response = await SendAsync(HttpMethod.Get, "api/Reservations");
            await EnsureSuccessAsync(response, "Failed to load reservations.");
            return await ReadFromJsonAsync<List<ReservationModel>>(response) ?? new List<ReservationModel>();
        }

        public async Task AddReservationAsync(ReservationModel reservation)
        {
            if (_currentUser != null && reservation.UserId == 0)
            {
                reservation.UserId = _currentUser.Id;
            }

            var response = await SendAsync(HttpMethod.Post, "api/Reservations", reservation);
            await EnsureSuccessAsync(response, "Failed to add reservation.");
            NotifyStateChanged();
        }

        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            var response = await SendAsync(HttpMethod.Delete, $"api/Reservations/{reservation.Id}");
            await EnsureSuccessAsync(response, $"Failed to remove reservation id={reservation.Id}.");
            NotifyStateChanged();
        }

        public async Task RemoveRecurringReservationAsync(ReservationModel reservation, string scope)
        {
            var response = await SendAsync(
                HttpMethod.Post,
                $"api/Reservations/{reservation.Id}/series-delete",
                new ReservationSeriesDeleteRequest
                {
                    OriginalReservation = reservation,
                    Scope = scope
                });

            await EnsureSuccessAsync(response, $"Failed to remove recurring reservation id={reservation.Id}.");
            NotifyStateChanged();
        }

        public async Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify)
        {
            var response = await SendAsync(
                HttpMethod.Put,
                $"api/Reservations/{reservation.Id}?notifyParticipants={shouldNotify.ToString().ToLowerInvariant()}",
                reservation);

            await EnsureSuccessAsync(response, $"Failed to update reservation id={reservation.Id}.");
            NotifyStateChanged();
        }

        public async Task UpdateRecurringReservationAsync(ReservationModel originalReservation, ReservationModel updatedReservation, bool shouldNotify, string scope)
        {
            var response = await SendAsync(
                HttpMethod.Post,
                $"api/Reservations/{originalReservation.Id}/series-update",
                new ReservationSeriesUpdateRequest
                {
                    OriginalReservation = originalReservation,
                    UpdatedReservation = updatedReservation,
                    NotifyParticipants = shouldNotify,
                    Scope = scope
                });

            await EnsureSuccessAsync(response, $"Failed to update recurring reservation id={originalReservation.Id}.");
            NotifyStateChanged();
        }

        public async Task JoinReservationAsync(int reservationId)
        {
            var response = await SendAsync(HttpMethod.Post, $"api/Reservations/{reservationId}/join");
            await EnsureSuccessAsync(response, $"Failed to join reservation id={reservationId}.");
            NotifyStateChanged();
        }

        public async Task LeaveReservationAsync(int reservationId)
        {
            var response = await SendAsync(HttpMethod.Post, $"api/Reservations/{reservationId}/leave");
            await EnsureSuccessAsync(response, $"Failed to leave reservation id={reservationId}.");
            NotifyStateChanged();
        }

        public async Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            var response = await SendAsync(HttpMethod.Get, $"api/Notifications/user/{userId}");
            await EnsureSuccessAsync(response, $"Failed to load notifications for user id={userId}.");
            return await ReadFromJsonAsync<List<NotificationModel>>(response) ?? new List<NotificationModel>();
        }

        public async Task AddNotificationAsync(NotificationModel notification)
        {
            var response = await SendAsync(HttpMethod.Post, "api/Notifications", notification);
            await EnsureSuccessAsync(response, "Failed to add notification.");
            NotifyStateChanged();
        }

        public async Task DeleteNotificationAsync(int notificationId)
        {
            var response = await SendAsync(HttpMethod.Delete, $"api/Notifications/{notificationId}");
            await EnsureSuccessAsync(response, $"Failed to delete notification id={notificationId}.");
            NotifyStateChanged();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            var response = await SendAsync(HttpMethod.Put, $"api/Notifications/{notificationId}/read");
            await EnsureSuccessAsync(response, $"Failed to mark notification id={notificationId} as read.");
            NotifyStateChanged();
        }

        private async Task<UserModel?> GetSessionUserAsync()
        {
            try
            {
                var response = await SendAsync(HttpMethod.Get, "api/Auth/me");

                if (response.StatusCode == HttpStatusCode.Unauthorized)
                {
                    return null;
                }

                await EnsureSuccessAsync(response, "Failed to restore the current session.");
                return await ReadFromJsonAsync<UserModel>(response);
            }
            catch
            {
                return null;
            }
        }

        private async Task<HttpResponseMessage> SendAsync(HttpMethod method, string requestUri, object? body = null)
        {
            var request = new HttpRequestMessage(method, requestUri);
            request.SetBrowserRequestCredentials(BrowserRequestCredentials.Include);

            if (body != null)
            {
                request.Content = JsonContent.Create(body);
            }

            return await _httpClient.SendAsync(request);
        }

        private static async Task<T?> ReadFromJsonAsync<T>(HttpResponseMessage response)
        {
            if (response.Content.Headers.ContentLength == 0)
            {
                return default;
            }

            return await response.Content.ReadFromJsonAsync<T>(JsonOptions);
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
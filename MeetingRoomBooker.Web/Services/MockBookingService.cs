using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using MeetingRoomBooker.Shared.Models;
using MeetingRoomBooker.Shared.Services;
using Microsoft.JSInterop;
using Microsoft.Extensions.Logging;

namespace MeetingRoomBooker.Web.Services
{
    public sealed class MockBookingService : IBookingService
    {
        private static List<ReservationModel> _reservations = new();
        private static List<UserModel> _users = new();
        private static List<NotificationModel> _notifications = new();
        private static int _nextId = 1;
        private static int _notifId = 1;
        private static bool _isLoaded = false;

        private readonly IJSRuntime _js;
        private readonly ILogger<MockBookingService> _logger;

        public UserModel? CurrentUser { get; private set; }
        public event Action? OnChange;

        public MockBookingService(IJSRuntime js, ILogger<MockBookingService> logger)
        {
            _js = js;
            _logger = logger;
        }

        private void NotifyStateChanged() => OnChange?.Invoke();

        private async Task EnsureLoaded()
        {
            if (_isLoaded) return;

            try
            {
                var uJson = await _js.InvokeAsync<string>("localStorage.getItem", "demo_users");
                if (!string.IsNullOrWhiteSpace(uJson))
                {
                    _users = JsonSerializer.Deserialize<List<UserModel>>(uJson) ?? new();
                }

                var rJson = await _js.InvokeAsync<string>("localStorage.getItem", "demo_reservations");
                if (!string.IsNullOrWhiteSpace(rJson))
                {
                    _reservations = JsonSerializer.Deserialize<List<ReservationModel>>(rJson) ?? new();
                }

                var nJson = await _js.InvokeAsync<string>("localStorage.getItem", "demo_notifications");
                if (!string.IsNullOrWhiteSpace(nJson))
                {
                    _notifications = JsonSerializer.Deserialize<List<NotificationModel>>(nJson) ?? new();
                }
            }
            catch (Exception ex)
            {
                _logger.LogWarning(
                    ex,
                    "Failed to load demo data from localStorage. The service will continue with in-memory defaults.");

                _users ??= new();
                _reservations ??= new();
                _notifications ??= new();
            }

            var usersChanged = EnsureDemoUsers();

            _nextId = _reservations.Any() ? _reservations.Max(r => r.Id) + 1 : 1;
            _notifId = _notifications.Any() ? _notifications.Max(n => n.Id) + 1 : 1;

            if (usersChanged)
            {
                await SaveData();
            }

            _isLoaded = true;
        }

        private bool EnsureDemoUsers()
        {
            var changed = false;

            changed |= UpsertDemoUser(
                email: "haru@demo.com",
                name: "Haru",
                password: "password",
                isAdmin: true,
                avatarColor: "#58a6ff");

            changed |= UpsertDemoUser(
                email: "demo@demo.com",
                name: "demo",
                password: "password",
                isAdmin: false,
                avatarColor: "#3fb950");

            changed |= UpsertDemoUser(
                email: "sato@demo.com",
                name: "Sato",
                password: "password",
                isAdmin: false,
                avatarColor: "#d29922");

            return changed;
        }

        private bool UpsertDemoUser(
            string email,
            string name,
            string password,
            bool isAdmin,
            string avatarColor)
        {
            var existing = _users.FirstOrDefault(u =>
                string.Equals(u.Email, email, StringComparison.OrdinalIgnoreCase));

            if (existing is null)
            {
                _users.Add(new UserModel
                {
                    Id = GetNextUserId(),
                    Name = name,
                    Email = email,
                    Password = password,
                    IsAdmin = isAdmin,
                    AvatarColor = avatarColor
                });

                return true;
            }

            var changed = false;

            if (!string.Equals(existing.Name, name, StringComparison.Ordinal))
            {
                existing.Name = name;
                changed = true;
            }

            if (!string.Equals(existing.Email, email, StringComparison.OrdinalIgnoreCase))
            {
                existing.Email = email;
                changed = true;
            }

            if (!string.Equals(existing.Password, password, StringComparison.Ordinal))
            {
                existing.Password = password;
                changed = true;
            }

            if (existing.IsAdmin != isAdmin)
            {
                existing.IsAdmin = isAdmin;
                changed = true;
            }

            if (string.IsNullOrWhiteSpace(existing.AvatarColor) || existing.AvatarColor != avatarColor)
            {
                existing.AvatarColor = avatarColor;
                changed = true;
            }

            if (existing.Id <= 0)
            {
                existing.Id = GetNextUserId();
                changed = true;
            }

            return changed;
        }

        private int GetNextUserId()
        {
            return _users.Any() ? _users.Max(u => u.Id) + 1 : 1;
        }

        private async Task SaveData()
        {
            try
            {
                await _js.InvokeVoidAsync("localStorage.setItem", "demo_users", JsonSerializer.Serialize(_users));
                await _js.InvokeVoidAsync("localStorage.setItem", "demo_reservations", JsonSerializer.Serialize(_reservations));
                await _js.InvokeVoidAsync("localStorage.setItem", "demo_notifications", JsonSerializer.Serialize(_notifications));
            }
            catch (Exception ex)
            {
                _logger.LogError(
                    ex,
                    "Failed to save demo data to localStorage.");
            }
        }

        public async Task<UserModel?> LoginAsync(string email, string password)
        {
            await EnsureLoaded();

            var normalizedEmail = email?.Trim() ?? string.Empty;
            var normalizedPassword = password ?? string.Empty;

            var user = _users.FirstOrDefault(u =>
                string.Equals(u.Email, normalizedEmail, StringComparison.OrdinalIgnoreCase) &&
                string.Equals(u.Password, normalizedPassword, StringComparison.Ordinal));

            if (user is null) return null;

            CurrentUser = user;
            NotifyStateChanged();
            return user;
        }

        public async Task<bool> RegisterUserAsync(UserModel user)
        {
            await EnsureLoaded();

            if (_users.Any(u => string.Equals(u.Email, user.Email, StringComparison.OrdinalIgnoreCase)))
            {
                return false;
            }

            user.Id = GetNextUserId();
            user.IsAdmin = false;

            if (string.IsNullOrWhiteSpace(user.AvatarColor))
            {
                user.AvatarColor = "#a371f7";
            }

            _users.Add(user);

            await SaveData();
            NotifyStateChanged();
            return true;
        }

        public async Task DeleteUserAsync(int userId)
        {
            await EnsureLoaded();

            var u = _users.FirstOrDefault(x => x.Id == userId);
            if (u is null) return;
            if (u.IsAdmin) return;

            _users.Remove(u);

            await SaveData();
            NotifyStateChanged();
        }

        public void Logout()
        {
            CurrentUser = null;
            NotifyStateChanged();
        }

        public UserModel? GetCurrentUser() => CurrentUser;

        public async Task<List<UserModel>> GetAllUsersAsync()
        {
            await EnsureLoaded();
            return _users.OrderBy(u => u.Id).ToList();
        }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            await EnsureLoaded();
            return _reservations;
        }

        public async Task AddReservationAsync(ReservationModel original)
        {
            await EnsureLoaded();

            var r = new ReservationModel
            {
                Id = _nextId++,
                Name = string.IsNullOrEmpty(original.Name) && CurrentUser != null ? CurrentUser.Name : original.Name,
                Room = original.Room,
                NumberOfPeople = original.NumberOfPeople,
                Type = original.Type,
                Purpose = original.Purpose,
                UserId = original.UserId == 0 && CurrentUser != null ? CurrentUser.Id : original.UserId,
                ParticipantIds = original.ParticipantIds != null ? new List<int>(original.ParticipantIds) : new List<int>(),
                RepeatType = original.RepeatType,
                RepeatUntil = original.RepeatUntil,
                Date = original.Date,
                StartTime = original.Date.Date + original.StartTime.TimeOfDay,
                EndTime = original.Date.Date + original.EndTime.TimeOfDay
            };

            var conflicts = _reservations
                .Where(x => x.Room == r.Room && x.Date.Date == r.Date.Date && x.Id != r.Id &&
                            ((r.StartTime >= x.StartTime && r.StartTime < x.EndTime) ||
                             (r.EndTime > x.StartTime && r.EndTime <= x.EndTime) ||
                             (r.StartTime <= x.StartTime && r.EndTime >= x.EndTime)))
                .ToList();

            foreach (var c in conflicts)
            {
                if (c.UserId != r.UserId)
                {
                    _notifications.Add(new NotificationModel
                    {
                        Id = _notifId++,
                        UserId = c.UserId,
                        Type = "Warning",
                        Message = $"【警告】{r.Date:MM/dd}の「{c.Purpose}」と時間が重複しました。",
                        TargetDate = r.Date,
                        TargetReservationId = r.Id,
                        IsRead = false
                    });
                }
            }

            _reservations.Add(r);

            foreach (var pid in r.ParticipantIds)
            {
                if (CurrentUser != null && pid == CurrentUser.Id) continue;

                if (IsRecurringReservation(r))
                {
                    UpsertRecurringReservationNotification(pid, r);
                    continue;
                }

                _notifications.Add(new NotificationModel
                {
                    Id = _notifId++,
                    UserId = pid,
                    Type = "Info",
                    Message = $"{r.Name}さんが予約「{r.Purpose}」を追加しました。",
                    TargetDate = r.Date,
                    TargetReservationId = r.Id,
                    IsRead = false
                });
            }

            await SaveData();
            NotifyStateChanged();
        }

        private bool IsRecurringReservation(ReservationModel reservation)
        {
            return !string.IsNullOrWhiteSpace(reservation.RepeatType) &&
                   reservation.RepeatType != "しない" &&
                   reservation.RepeatUntil.HasValue;
        }

        private List<ReservationModel> GetRecurringSeriesReservations(ReservationModel reservation)
        {
            return _reservations
                .Where(x =>
                    x.UserId == reservation.UserId &&
                    x.Name == reservation.Name &&
                    x.Room == reservation.Room &&
                    x.Type == reservation.Type &&
                    x.Purpose == reservation.Purpose &&
                    x.RepeatType == reservation.RepeatType &&
                    x.RepeatUntil?.Date == reservation.RepeatUntil?.Date &&
                    x.StartTime.TimeOfDay == reservation.StartTime.TimeOfDay &&
                    x.EndTime.TimeOfDay == reservation.EndTime.TimeOfDay)
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .ToList();
        }

        private void UpsertRecurringReservationNotification(int participantId, ReservationModel reservation)
        {
            var seriesReservations = GetRecurringSeriesReservations(reservation);
            var firstReservation = seriesReservations.FirstOrDefault();

            if (firstReservation is null) return;

            var count = seriesReservations.Count;
            var existing = _notifications.FirstOrDefault(n =>
                n.UserId == participantId &&
                n.Type == "Info" &&
                n.TargetReservationId == firstReservation.Id);

            var message = $"{reservation.Name}さんが繰り返し予約「{reservation.Purpose}」を追加しました。({count}件)";

            if (existing is null)
            {
                _notifications.Add(new NotificationModel
                {
                    Id = _notifId++,
                    UserId = participantId,
                    Type = "Info",
                    Message = message,
                    TargetDate = firstReservation.Date,
                    TargetReservationId = firstReservation.Id,
                    IsRead = false
                });

                return;
            }

            existing.Message = message;
            existing.IsRead = false;
        }

        public async Task RemoveReservationAsync(ReservationModel r)
        {
            await EnsureLoaded();

            var target = _reservations.FirstOrDefault(x => x.Id == r.Id);
            if (target is null) return;

            _reservations.Remove(target);

            foreach (var pid in r.ParticipantIds)
            {
                if (CurrentUser != null && pid == CurrentUser.Id) continue;

                _notifications.Add(new NotificationModel
                {
                    Id = _notifId++,
                    UserId = pid,
                    Type = "Info",
                    Message = $"予約「{r.Purpose}」が削除されました。",
                    TargetDate = r.Date,
                    TargetReservationId = r.Id,
                    IsRead = false
                });
            }

            await SaveData();
            NotifyStateChanged();
        }

        public async Task UpdateReservationAsync(ReservationModel r, bool shouldNotify)
        {
            await EnsureLoaded();

            var index = _reservations.FindIndex(x => x.Id == r.Id);
            if (index == -1) return;

            if (shouldNotify)
            {
                foreach (var pid in r.ParticipantIds)
                {
                    if (CurrentUser != null && pid == CurrentUser.Id) continue;

                    _notifications.Add(new NotificationModel
                    {
                        Id = _notifId++,
                        UserId = pid,
                        Type = "Info",
                        Message = $"予約「{r.Purpose}」の内容が変更されました。",
                        TargetDate = r.Date,
                        TargetReservationId = r.Id,
                        IsRead = false
                    });
                }
            }

            r.StartTime = r.Date.Date + r.StartTime.TimeOfDay;
            r.EndTime = r.Date.Date + r.EndTime.TimeOfDay;

            _reservations[index] = r;
            await SaveData();
            NotifyStateChanged();
        }

        public async Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            await EnsureLoaded();
            return _notifications
                .Where(n => n.UserId == userId)
                .OrderByDescending(n => n.CreatedAt)
                .ToList();
        }

        public async Task AddNotificationAsync(NotificationModel notification)
        {
            await EnsureLoaded();

            notification.Id = _notifId++;
            _notifications.Add(notification);

            await SaveData();
            NotifyStateChanged();
        }

        public async Task DeleteNotificationAsync(int notificationId)
        {
            await EnsureLoaded();

            var n = _notifications.FirstOrDefault(x => x.Id == notificationId);
            if (n is null) return;

            _notifications.Remove(n);
            await SaveData();
            NotifyStateChanged();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            await EnsureLoaded();

            var n = _notifications.FirstOrDefault(x => x.Id == notificationId);
            if (n is null) return;

            n.IsRead = true;
            await SaveData();
            NotifyStateChanged();
        }
    }
}
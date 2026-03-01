using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using MeetingRoomBooker.Shared.Models;
using MeetingRoomBooker.Shared.Services;
using Microsoft.JSInterop;

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

        public UserModel? CurrentUser { get; private set; }
        public event Action? OnChange;

        public MockBookingService(IJSRuntime js) => _js = js;

        private void NotifyStateChanged() => OnChange?.Invoke();

        private async Task EnsureLoaded()
        {
            if (_isLoaded) return;

            try
            {
                var uJson = await _js.InvokeAsync<string>("localStorage.getItem", "demo_users");
                if (!string.IsNullOrEmpty(uJson))
                {
                    _users = JsonSerializer.Deserialize<List<UserModel>>(uJson) ?? new();
                }

                var rJson = await _js.InvokeAsync<string>("localStorage.getItem", "demo_reservations");
                if (!string.IsNullOrEmpty(rJson))
                {
                    _reservations = JsonSerializer.Deserialize<List<ReservationModel>>(rJson) ?? new();
                }

                var nJson = await _js.InvokeAsync<string>("localStorage.getItem", "demo_notifications");
                if (!string.IsNullOrEmpty(nJson))
                {
                    _notifications = JsonSerializer.Deserialize<List<NotificationModel>>(nJson) ?? new();
                }

                _nextId = _reservations.Count > 0 ? _reservations.Max(r => r.Id) + 1 : 1;
                _notifId = _notifications.Count > 0 ? _notifications.Max(n => n.Id) + 1 : 1;
            }
            catch
            {
            }

            if (!_users.Any())
            {
                _users.AddRange(new[]
                {
                    new UserModel { Id = 1, Name = "Haru", Email = "haru@demo.com", Password = "password", IsAdmin = true,  AvatarColor = "#58a6ff" },
                    new UserModel { Id = 2, Name = "demo", Email = "demo@demo.com", Password = "password", IsAdmin = false, AvatarColor = "#3fb950" },
                    new UserModel { Id = 3, Name = "Sato", Email = "sato@demo.com", Password = "password", IsAdmin = false, AvatarColor = "#d29922" },
                });

                await SaveData();
            }

            _isLoaded = true;
        }

        private async Task SaveData()
        {
            try
            {
                await _js.InvokeVoidAsync("localStorage.setItem", "demo_users", JsonSerializer.Serialize(_users));
                await _js.InvokeVoidAsync("localStorage.setItem", "demo_reservations", JsonSerializer.Serialize(_reservations));
                await _js.InvokeVoidAsync("localStorage.setItem", "demo_notifications", JsonSerializer.Serialize(_notifications));
            }
            catch
            {
            }
        }

        public async Task<UserModel?> LoginAsync(string email, string password)
        {
            await EnsureLoaded();

            var user = _users.FirstOrDefault(u => u.Email == email && u.Password == password);
            if (user is null) return null;

            CurrentUser = user;
            NotifyStateChanged();
            return user;
        }

        public async Task<bool> RegisterUserAsync(UserModel user)
        {
            await EnsureLoaded();

            user.Id = _users.Any() ? _users.Max(u => u.Id) + 1 : 1;
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
            return _users;
        }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            await EnsureLoaded();
            return _reservations;
        }

        public async Task AddReservationAsync(ReservationModel original)
        {
            await EnsureLoaded();

            var targetDates = new List<DateTime> { original.Date };

            if (original.RepeatUntil.HasValue && original.RepeatUntil.Value.Date > original.Date.Date)
            {
                var nextDate = original.Date;

                if (original.RepeatType == "毎日")
                {
                    nextDate = nextDate.AddDays(1);
                    while (nextDate.Date <= original.RepeatUntil.Value.Date)
                    {
                        targetDates.Add(nextDate);
                        nextDate = nextDate.AddDays(1);
                    }
                }
                else if (original.RepeatType == "毎週")
                {
                    nextDate = nextDate.AddDays(7);
                    while (nextDate.Date <= original.RepeatUntil.Value.Date)
                    {
                        targetDates.Add(nextDate);
                        nextDate = nextDate.AddDays(7);
                    }
                }
            }

            foreach (var date in targetDates)
            {
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
                    Date = date,
                    StartTime = date.Date + original.StartTime.TimeOfDay,
                    EndTime = date.Date + original.EndTime.TimeOfDay
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
            }

            await SaveData();
            NotifyStateChanged();
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
            return _notifications.Where(n => n.UserId == userId).OrderByDescending(n => n.CreatedAt).ToList();
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
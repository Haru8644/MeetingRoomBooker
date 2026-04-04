using System.Text.Json;
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
            if (_isLoaded)
            {
                return;
            }

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

            if (user is null)
            {
                return null;
            }

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
            user.Name = user.Name?.Trim() ?? string.Empty;
            user.Email = user.Email?.Trim() ?? string.Empty;
            user.ChatworkAccountId = NormalizeChatworkAccountId(user.ChatworkAccountId);

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

            var user = _users.FirstOrDefault(x => x.Id == userId);
            if (user is null || user.IsAdmin)
            {
                return;
            }

            _users.Remove(user);

            await SaveData();
            NotifyStateChanged();
        }

        public async Task UpdateUserNameAsync(int userId, string name)
        {
            await EnsureLoaded();

            var user = _users.FirstOrDefault(x => x.Id == userId);
            if (user is null)
            {
                return;
            }

            var normalizedName = name?.Trim() ?? string.Empty;
            if (string.IsNullOrWhiteSpace(normalizedName))
            {
                return;
            }

            user.Name = normalizedName;

            foreach (var reservation in _reservations.Where(x => x.UserId == userId))
            {
                reservation.Name = normalizedName;
            }

            if (CurrentUser?.Id == userId)
            {
                CurrentUser.Name = normalizedName;
            }

            await SaveData();
            NotifyStateChanged();
        }

        public async Task UpdateUserChatworkAccountIdAsync(int userId, string? chatworkAccountId)
        {
            await EnsureLoaded();

            var user = _users.FirstOrDefault(x => x.Id == userId);
            if (user is null)
            {
                return;
            }

            user.ChatworkAccountId = NormalizeChatworkAccountId(chatworkAccountId);

            if (CurrentUser?.Id == userId)
            {
                CurrentUser.ChatworkAccountId = user.ChatworkAccountId;
            }

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

            var reservation = CreateReservationSnapshot(original);
            _reservations.Add(reservation);

            NotifyConflictOwners(reservation);
            NotifyParticipantsForCreatedReservation(reservation);

            await SaveData();
            NotifyStateChanged();
        }

        private bool IsRecurringReservation(ReservationModel reservation)
        {
            return !string.IsNullOrWhiteSpace(reservation.RepeatType)
                   && reservation.RepeatType != "しない"
                   && reservation.RepeatUntil.HasValue;
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

            if (firstReservation is null)
            {
                return;
            }

            var count = seriesReservations.Count;
            UpsertNotification(
                participantId,
                "Info",
                $"{reservation.Name}さんが繰り返し予約「{GetReservationLabel(reservation)}」にあなたを追加しました。({count}件)",
                firstReservation.Date,
                firstReservation.Id,
                GetRecurringNotificationPrefix(reservation));
        }

        private ReservationModel CreateReservationSnapshot(ReservationModel original)
        {
            var userId = original.UserId == 0 && CurrentUser != null ? CurrentUser.Id : original.UserId;

            return new ReservationModel
            {
                Id = original.Id > 0 ? original.Id : _nextId++,
                Name = string.IsNullOrEmpty(original.Name) && CurrentUser != null ? CurrentUser.Name : original.Name,
                Room = original.Room,
                NumberOfPeople = original.NumberOfPeople,
                Type = original.Type,
                Purpose = original.Purpose,
                UserId = userId,
                ParticipantIds = (original.ParticipantIds ?? new List<int>())
                    .Where(id => id > 0)
                    .Distinct()
                    .ToList(),
                RepeatType = string.IsNullOrWhiteSpace(original.RepeatType) ? "しない" : original.RepeatType,
                RepeatUntil = string.IsNullOrWhiteSpace(original.RepeatType) || original.RepeatType == "しない"
                    ? null
                    : original.RepeatUntil,
                Date = original.Date,
                StartTime = original.Date.Date + original.StartTime.TimeOfDay,
                EndTime = original.Date.Date + original.EndTime.TimeOfDay
            };
        }

        private List<int> GetNotifiableParticipantIds(ReservationModel reservation)
        {
            return (reservation.ParticipantIds ?? new List<int>())
                .Where(id => id > 0 && id != reservation.UserId)
                .Distinct()
                .ToList();
        }

        private List<ReservationModel> GetConflictingReservations(ReservationModel reservation)
        {
            return _reservations
                .Where(x =>
                    x.Id != reservation.Id &&
                    x.Room == reservation.Room &&
                    x.Date.Date == reservation.Date.Date &&
                    x.StartTime < reservation.EndTime &&
                    x.EndTime > reservation.StartTime)
                .ToList();
        }

        private void NotifyConflictOwners(ReservationModel reservation)
        {
            foreach (var conflict in GetConflictingReservations(reservation))
            {
                if (conflict.UserId == reservation.UserId)
                {
                    continue;
                }

                UpsertNotification(
                    conflict.UserId,
                    "Warning",
                    $"【警告】{reservation.Date:MM/dd}の「{reservation.Purpose}」と時間が重複しました。",
                    reservation.Date,
                    reservation.Id);
            }
        }

        private void NotifyParticipantsForCreatedReservation(ReservationModel reservation)
        {
            var participantIds = GetNotifiableParticipantIds(reservation);
            foreach (var participantId in participantIds)
            {
                if (IsRecurringReservation(reservation))
                {
                    UpsertRecurringReservationNotification(participantId, reservation);
                    continue;
                }

                UpsertNotification(
                    participantId,
                    "Info",
                    BuildAddedParticipantMessage(reservation),
                    reservation.Date,
                    reservation.Id);
            }
        }

        private void NotifyParticipantsForUpdatedReservation(
            ReservationModel previousReservation,
            ReservationModel reservation,
            IReadOnlyCollection<int> previousParticipantIds,
            bool shouldNotify)
        {
            if (!shouldNotify)
            {
                return;
            }

            var currentParticipantIds = GetNotifiableParticipantIds(reservation);
            var retainedParticipantIds = currentParticipantIds.Intersect(previousParticipantIds).ToList();
            var addedParticipantIds = currentParticipantIds.Except(previousParticipantIds).ToList();
            var removedParticipantIds = previousParticipantIds.Except(currentParticipantIds).ToList();
            var detailedMessage = BuildReservationUpdatedMessage(previousReservation, reservation, addedParticipantIds, removedParticipantIds);

            foreach (var participantId in addedParticipantIds)
            {
                UpsertNotification(
                    participantId,
                    "Info",
                    BuildAddedParticipantMessage(reservation),
                    reservation.Date,
                    reservation.Id);
            }

            foreach (var participantId in removedParticipantIds)
            {
                UpsertNotification(
                    participantId,
                    "Info",
                    BuildRemovedParticipantMessage(reservation),
                    reservation.Date,
                    reservation.Id);
            }

            if (string.IsNullOrWhiteSpace(detailedMessage))
            {
                return;
            }

            foreach (var participantId in retainedParticipantIds)
            {
                UpsertNotification(
                    participantId,
                    "Info",
                    detailedMessage,
                    reservation.Date,
                    reservation.Id);
            }
        }

        private string GetReservationLabel(ReservationModel reservation)
        {
            return string.IsNullOrWhiteSpace(reservation.Purpose)
                ? reservation.Name
                : reservation.Purpose;
        }

        private string GetTimeRangeText(ReservationModel reservation)
        {
            return $"{reservation.StartTime:HH:mm}〜{reservation.EndTime:HH:mm}";
        }

        private string BuildAddedParticipantMessage(ReservationModel reservation)
        {
            return $"{reservation.Name}さんが予約「{GetReservationLabel(reservation)}」の参加メンバーにあなたを追加しました。利用日: {reservation.Date:yyyy/MM/dd} / 会議室: {reservation.Room} / 時間: {GetTimeRangeText(reservation)}";
        }

        private string BuildRemovedParticipantMessage(ReservationModel reservation)
        {
            return $"{reservation.Name}さんが予約「{GetReservationLabel(reservation)}」の参加メンバーからあなたを削除しました。";
        }

        private string GetRecurringNotificationPrefix(ReservationModel reservation)
        {
            return $"{reservation.Name}さんが繰り返し予約「{GetReservationLabel(reservation)}」にあなたを追加しました。(";
        }

        private string BuildReservationUpdatedMessage(
            ReservationModel previousReservation,
            ReservationModel reservation,
            IReadOnlyCollection<int> addedParticipantIds,
            IReadOnlyCollection<int> removedParticipantIds)
        {
            var changes = new List<string>();

            if (previousReservation.Date.Date != reservation.Date.Date)
            {
                changes.Add($"利用日が {previousReservation.Date:yyyy/MM/dd} から {reservation.Date:yyyy/MM/dd} に変更されました");
            }

            if (!string.Equals(previousReservation.Room, reservation.Room, StringComparison.Ordinal))
            {
                changes.Add($"会議室が {previousReservation.Room} から {reservation.Room} に変更されました");
            }

            if (!string.Equals(previousReservation.Type, reservation.Type, StringComparison.Ordinal))
            {
                changes.Add($"区分が {previousReservation.Type} から {reservation.Type} に変更されました");
            }

            if (previousReservation.StartTime.TimeOfDay != reservation.StartTime.TimeOfDay
                || previousReservation.EndTime.TimeOfDay != reservation.EndTime.TimeOfDay)
            {
                changes.Add($"時間が {GetTimeRangeText(previousReservation)} から {GetTimeRangeText(reservation)} に変更されました");
            }

            var previousLabel = GetReservationLabel(previousReservation);
            var currentLabel = GetReservationLabel(reservation);
            if (!string.Equals(previousLabel, currentLabel, StringComparison.Ordinal))
            {
                changes.Add($"予約名が「{previousLabel}」から「{currentLabel}」に変更されました");
            }

            var addedNames = FormatUserNames(addedParticipantIds);
            if (!string.IsNullOrWhiteSpace(addedNames))
            {
                changes.Add($"参加メンバーに {addedNames} が追加されました");
            }

            var removedNames = FormatUserNames(removedParticipantIds);
            if (!string.IsNullOrWhiteSpace(removedNames))
            {
                changes.Add($"参加メンバーから {removedNames} が削除されました");
            }

            if (changes.Count == 0)
            {
                return string.Empty;
            }

            var notificationLabel = !string.IsNullOrWhiteSpace(previousLabel) ? previousLabel : currentLabel;
            return $"予約「{notificationLabel}」が更新されました。{string.Join(" ", changes.Select(change => $"{change}。"))}";
        }

        private string FormatUserNames(IEnumerable<int> userIds)
        {
            var names = userIds
                .Distinct()
                .Select(id => _users.FirstOrDefault(user => user.Id == id)?.Name ?? $"ユーザー{id}")
                .Where(name => !string.IsNullOrWhiteSpace(name))
                .ToList();

            return string.Join("、", names);
        }

        private void UpsertNotification(
            int userId,
            string type,
            string message,
            DateTime targetDate,
            int targetReservationId,
            string? messagePrefix = null)
        {
            var candidates = _notifications
                .Where(notification =>
                    notification.UserId == userId &&
                    notification.Type == type &&
                    notification.TargetReservationId == targetReservationId)
                .OrderByDescending(notification => notification.CreatedAt)
                .ToList();

            var matchingNotifications = string.IsNullOrWhiteSpace(messagePrefix)
                ? candidates.Where(notification => notification.Message == message).ToList()
                : candidates.Where(notification => notification.Message.StartsWith(messagePrefix, StringComparison.Ordinal)).ToList();

            var existing = matchingNotifications.FirstOrDefault();

            if (existing is null)
            {
                _notifications.Add(new NotificationModel
                {
                    Id = _notifId++,
                    UserId = userId,
                    Type = type,
                    Message = message,
                    TargetDate = targetDate,
                    TargetReservationId = targetReservationId,
                    CreatedAt = DateTime.Now,
                    IsRead = false
                });

                return;
            }

            existing.Message = message;
            existing.TargetDate = targetDate;
            existing.CreatedAt = DateTime.Now;
            existing.IsRead = false;

            if (matchingNotifications.Count > 1)
            {
                foreach (var duplicate in matchingNotifications.Skip(1))
                {
                    _notifications.Remove(duplicate);
                }
            }
        }

        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            await EnsureLoaded();
            RemoveReservationCore(reservation);
            await SaveData();
            NotifyStateChanged();
        }

        public async Task RemoveRecurringReservationAsync(ReservationModel reservation, string scope)
        {
            await EnsureLoaded();

            var targets = GetRecurringTargets(reservation, scope);
            foreach (var target in targets)
            {
                RemoveReservationCore(target);
            }

            await SaveData();
            NotifyStateChanged();
        }

        public async Task UpdateReservationAsync(ReservationModel reservation, bool shouldNotify)
        {
            await EnsureLoaded();
            UpdateReservationCore(reservation, shouldNotify);
            await SaveData();
            NotifyStateChanged();
        }

        public async Task UpdateRecurringReservationAsync(ReservationModel originalReservation, ReservationModel updatedReservation, bool shouldNotify, string scope)
        {
            await EnsureLoaded();

            var targets = GetRecurringTargets(originalReservation, scope);
            var dayOffset = updatedReservation.Date.Date - originalReservation.Date.Date;

            foreach (var target in targets)
            {
                var nextDate = target.Date.Date.Add(dayOffset);
                var nextReservation = CreateReservationSnapshot(target);
                nextReservation.Name = updatedReservation.Name;
                nextReservation.Room = updatedReservation.Room;
                nextReservation.Type = updatedReservation.Type;
                nextReservation.Purpose = updatedReservation.Purpose;
                nextReservation.Date = nextDate;
                nextReservation.StartTime = nextDate + updatedReservation.StartTime.TimeOfDay;
                nextReservation.EndTime = nextDate + updatedReservation.EndTime.TimeOfDay;
                nextReservation.ParticipantIds = (updatedReservation.ParticipantIds ?? new List<int>()).Distinct().ToList();

                UpdateReservationCore(nextReservation, shouldNotify);
            }

            await SaveData();
            NotifyStateChanged();
        }

        public async Task JoinReservationAsync(int reservationId)
        {
            await EnsureLoaded();

            if (CurrentUser is null)
            {
                return;
            }

            var reservation = _reservations.FirstOrDefault(x => x.Id == reservationId);
            if (reservation is null)
            {
                return;
            }

            reservation.ParticipantIds ??= new List<int>();
            if (!reservation.ParticipantIds.Contains(CurrentUser.Id))
            {
                reservation.ParticipantIds.Add(CurrentUser.Id);
            }

            await SaveData();
            NotifyStateChanged();
        }

        public async Task LeaveReservationAsync(int reservationId)
        {
            await EnsureLoaded();

            if (CurrentUser is null)
            {
                return;
            }

            var reservation = _reservations.FirstOrDefault(x => x.Id == reservationId);
            if (reservation is null || reservation.UserId == CurrentUser.Id)
            {
                return;
            }

            reservation.ParticipantIds?.Remove(CurrentUser.Id);
            await SaveData();
            NotifyStateChanged();
        }

        private void RemoveReservationCore(ReservationModel reservation)
        {
            var target = _reservations.FirstOrDefault(x => x.Id == reservation.Id);
            if (target is null)
            {
                return;
            }

            _reservations.Remove(target);

            foreach (var participantId in reservation.ParticipantIds)
            {
                if (CurrentUser != null && participantId == CurrentUser.Id)
                {
                    continue;
                }

                _notifications.Add(new NotificationModel
                {
                    Id = _notifId++,
                    UserId = participantId,
                    Type = "Info",
                    Message = $"予約「{reservation.Purpose}」が削除されました。",
                    TargetDate = reservation.Date,
                    TargetReservationId = reservation.Id,
                    IsRead = false
                });
            }
        }

        private void UpdateReservationCore(ReservationModel reservation, bool shouldNotify)
        {
            var index = _reservations.FindIndex(x => x.Id == reservation.Id);
            if (index == -1)
            {
                return;
            }

            var previousReservation = _reservations[index];
            var previousParticipantIds = GetNotifiableParticipantIds(previousReservation);
            var previousReservationSnapshot = CreateReservationSnapshot(previousReservation);
            var nextReservation = CreateReservationSnapshot(reservation);
            nextReservation.Id = previousReservation.Id;
            nextReservation.UserId = previousReservation.UserId;

            _reservations[index] = nextReservation;

            NotifyParticipantsForUpdatedReservation(previousReservationSnapshot, nextReservation, previousParticipantIds, shouldNotify);
            NotifyConflictOwners(nextReservation);
        }

        private List<ReservationModel> GetRecurringTargets(ReservationModel reservation, string scope)
        {
            var matches = GetRecurringSeriesReservations(reservation);

            if (scope == ReservationSeriesScopes.Following)
            {
                return matches
                    .Where(x => x.Date.Date >= reservation.Date.Date)
                    .OrderBy(x => x.Date)
                    .ThenBy(x => x.Id)
                    .ToList();
            }

            return scope == ReservationSeriesScopes.All
                ? matches
                : matches.Where(x => x.Id == reservation.Id).ToList();
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

            var notification = _notifications.FirstOrDefault(x => x.Id == notificationId);
            if (notification is null)
            {
                return;
            }

            _notifications.Remove(notification);
            await SaveData();
            NotifyStateChanged();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            await EnsureLoaded();

            var notification = _notifications.FirstOrDefault(x => x.Id == notificationId);
            if (notification is null)
            {
                return;
            }

            notification.IsRead = true;
            await SaveData();
            NotifyStateChanged();
        }

        private static string? NormalizeChatworkAccountId(string? chatworkAccountId)
        {
            return string.IsNullOrWhiteSpace(chatworkAccountId)
                ? null
                : chatworkAccountId.Trim();
        }
    }
}
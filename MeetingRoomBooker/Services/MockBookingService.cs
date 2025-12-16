using MeetingRoomBooker.Models;

namespace MeetingRoomBooker.Services
{
    public class MockBookingService : IBookingService
    {
        private static List<ReservationModel> _reservations = new();
        private static List<UserModel> _users = new();
        private static List<NotificationModel> _notifications = new();
        private UserModel? _currentUser;
        public event Action OnChange;
        private void NotifyStateChanged() => OnChange?.Invoke();

        public MockBookingService()
        {
            if (!_users.Any())
            {
                _users.Add(new UserModel { Id = 1, Name = "Haru", Email = "haru@demo.com", Password = "password", AvatarColor = "#58a6ff" });
                _users.Add(new UserModel { Id = 2, Name = "Tanaka", Email = "tanaka@demo.com", Password = "password", AvatarColor = "#3fb950" });
                _users.Add(new UserModel { Id = 3, Name = "Suzuki", Email = "suzuki@demo.com", Password = "password", AvatarColor = "#a371f7" });
            }
        }

        public Task<List<ReservationModel>> GetReservationsAsync() => Task.FromResult(_reservations);

        public Task AddReservationAsync(ReservationModel reservation)
        {
            int newId = _reservations.Any() ? _reservations.Max(r => r.Id) + 1 : 1;
            reservation.Id = newId;
            if (_currentUser != null) reservation.UserId = _currentUser.Id;

            _reservations.Add(reservation);

            foreach (var partId in reservation.ParticipantIds)
            {
                var timeStr = $"{reservation.Date:MM/dd} {reservation.StartTime:HH:mm}～";
                _notifications.Add(new NotificationModel
                {
                    Id = _notifications.Any() ? _notifications.Max(n => n.Id) + 1 : 1,
                    UserId = partId,
                    Message = $"{timeStr} {reservation.Room} の予約に追加されました。",
                    Type = "Info",
                    TargetReservationId = newId,
                    TargetDate = reservation.Date
                });
            }

            NotifyStateChanged();
            return Task.CompletedTask;
        }

        public Task RemoveReservationAsync(ReservationModel reservation)
        {
            var target = _reservations.FirstOrDefault(r => r.Id == reservation.Id);
            if (target != null) _reservations.Remove(target);
            NotifyStateChanged();
            return Task.CompletedTask;
        }

        public Task UpdateReservationAsync(ReservationModel reservation)
        {
            var target = _reservations.FirstOrDefault(r => r.Id == reservation.Id);
            if (target != null)
            {
                target.Name = reservation.Name;
                target.Room = reservation.Room;
                target.NumberOfPeople = reservation.NumberOfPeople;
                target.Date = reservation.Date;
                target.StartTime = reservation.StartTime;
                target.EndTime = reservation.EndTime;
                target.Type = reservation.Type;
                target.Purpose = reservation.Purpose;
                target.ParticipantIds = reservation.ParticipantIds;
            }
            NotifyStateChanged();
            return Task.CompletedTask;
        }

        public Task<UserModel?> LoginAsync(string email, string password)
        {
            var user = _users.FirstOrDefault(u => u.Email == email && u.Password == password);
            if (user != null) _currentUser = user;
            NotifyStateChanged();
            return Task.FromResult(user);
        }

        public Task RegisterUserAsync(UserModel user)
        {
            if (_users.Any(u => u.Email == user.Email)) throw new Exception("登録済みのEmailです");
            int newId = _users.Any() ? _users.Max(u => u.Id) + 1 : 1;
            user.Id = newId;
            _users.Add(user);
            _currentUser = user;
            NotifyStateChanged();
            return Task.CompletedTask;
        }

        public Task<List<UserModel>> GetAllUsersAsync() => Task.FromResult(_users);
        public UserModel? GetCurrentUser() => _currentUser;
        public void Logout() { _currentUser = null; NotifyStateChanged(); }
        public Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            return Task.FromResult(_notifications.Where(n => n.UserId == userId).OrderByDescending(n => n.CreatedAt).ToList());
        }

        public Task AddNotificationAsync(NotificationModel notification)
        {
            int newId = _notifications.Any() ? _notifications.Max(n => n.Id) + 1 : 1;
            notification.Id = newId;
            _notifications.Add(notification);
            NotifyStateChanged();
            return Task.CompletedTask;
        }

        public Task MarkNotificationAsReadAsync(int notificationId)
        {
            var target = _notifications.FirstOrDefault(n => n.Id == notificationId);
            if (target != null)
            {
                target.IsRead = true;
                NotifyStateChanged(); 
            }
            return Task.CompletedTask;
        }
    }
}
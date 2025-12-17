using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Models;

namespace MeetingRoomBooker.Services
{
    public class DbBookingService : IBookingService
    {
        private readonly IDbContextFactory<AppDbContext> _contextFactory;
        private UserModel? _currentUser;
        public event Action? OnChange;
        private void NotifyStateChanged() => OnChange?.Invoke();

        public DbBookingService(IDbContextFactory<AppDbContext> contextFactory)
        {
            _contextFactory = contextFactory;
        }

        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            await context.Database.EnsureCreatedAsync();
            return await context.Reservations.ToListAsync();
        }

        public async Task AddReservationAsync(ReservationModel reservation)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            if (_currentUser != null) reservation.UserId = _currentUser.Id;

            context.Reservations.Add(reservation);
            await context.SaveChangesAsync();

            if (reservation.ParticipantIds != null && reservation.ParticipantIds.Any())
            {
                var timeStr = $"{reservation.Date:MM/dd} {reservation.StartTime:HH:mm}～";
                foreach (var partId in reservation.ParticipantIds)
                {
                    context.Notifications.Add(new NotificationModel
                    {
                        UserId = partId,
                        Message = $"{timeStr} {reservation.Room} の予約に追加されました。",
                        Type = "Info",
                        TargetReservationId = reservation.Id,
                        TargetDate = reservation.Date
                    });
                }
                await context.SaveChangesAsync();
            }
            NotifyStateChanged();
        }

        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var target = await context.Reservations.FindAsync(reservation.Id);
            if (target != null)
            {
                context.Reservations.Remove(target);
                await context.SaveChangesAsync();
                NotifyStateChanged();
            }
        }

        public async Task UpdateReservationAsync(ReservationModel reservation)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var target = await context.Reservations.FindAsync(reservation.Id);
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
                target.RepeatType = reservation.RepeatType;
                target.RepeatUntil = reservation.RepeatUntil;

                await context.SaveChangesAsync();
                NotifyStateChanged();
            }
        }

        public async Task<UserModel?> LoginAsync(string email, string password)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            await context.Database.EnsureCreatedAsync();
            var user = await context.Users.FirstOrDefaultAsync(u => u.Email == email && u.Password == password);
            if (user != null)
            {
                _currentUser = user;
                NotifyStateChanged();
            }
            return user;
        }

        public async Task RegisterUserAsync(UserModel user)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            await context.Database.EnsureCreatedAsync();
            if (await context.Users.AnyAsync(u => u.Email == user.Email))
            {
                throw new Exception("このメールアドレスは既に登録されています。");
            }

            context.Users.Add(user);
            await context.SaveChangesAsync();
            _currentUser = user;
            NotifyStateChanged();
        }

        public async Task<List<UserModel>> GetAllUsersAsync()
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            await context.Database.EnsureCreatedAsync();
            return await context.Users.ToListAsync();
        }

        public UserModel? GetCurrentUser() => _currentUser;

        public void Logout()
        {
            _currentUser = null;
            NotifyStateChanged();
        }

        public async Task<List<NotificationModel>> GetNotificationsAsync(int userId)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            await context.Database.EnsureCreatedAsync();
            return await context.Notifications
                .Where(n => n.UserId == userId)
                .OrderByDescending(n => n.CreatedAt)
                .ToListAsync();
        }

        public async Task AddNotificationAsync(NotificationModel notification)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            context.Notifications.Add(notification);
            await context.SaveChangesAsync();
            NotifyStateChanged();
        }

        public async Task MarkNotificationAsReadAsync(int notificationId)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var target = await context.Notifications.FindAsync(notificationId);
            if (target != null)
            {
                target.IsRead = true;
                await context.SaveChangesAsync();
                NotifyStateChanged();
            }
        }
    }
}
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.Reservations;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Tests;

public sealed class WorkScheduleNotificationServiceTests
{
    [Fact]
    public async Task CreateEntryAsync_CreatesSystemNotificationsForParticipants()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = CreateWorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
            EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
            ParticipantIds = new List<int> { 2 }
        };

        var result = await service.CreateEntryAsync(
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.NotNull(result.Entry);

        var notifications = await database.Context.Notifications
            .OrderBy(notification => notification.UserId)
            .ToListAsync();

        Assert.Equal(2, notifications.Count);

        Assert.Collection(
            notifications,
            notification =>
            {
                Assert.Equal(1, notification.UserId);
                Assert.Equal("Info", notification.Type);
                Assert.Equal(result.Entry.Id, notification.TargetWorkScheduleEntryId);
                Assert.Null(notification.TargetReservationId);
                Assert.Contains("社外予定「顧客訪問」が登録されました", notification.Message);
            },
            notification =>
            {
                Assert.Equal(2, notification.UserId);
                Assert.Equal("Info", notification.Type);
                Assert.Equal(result.Entry.Id, notification.TargetWorkScheduleEntryId);
                Assert.Null(notification.TargetReservationId);
                Assert.Contains("社外予定「顧客訪問」が登録されました", notification.Message);
            });
    }

    [Fact]
    public async Task NotifySeriesCreatedAsync_CreatesSummaryNotificationsForParticipants()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var notificationService = new WorkScheduleNotificationService(
            database.Context,
            new WorkScheduleParticipantConflictService(database.Context));

        var entries = new List<WorkScheduleEntryModel>
        {
            new()
            {
                Id = 10,
                CreatedByUserId = 1,
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                SeriesId = "series-1",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 6, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 6, 1, 11, 0, 0),
                RepeatType = WorkScheduleRepeatTypes.Weekly,
                RepeatUntil = new DateTime(2026, 6, 8),
                ParticipantIds = new List<int> { 1, 2 },
                Participants = "稲生遥希、田中太郎"
            },
            new()
            {
                Id = 11,
                CreatedByUserId = 1,
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                SeriesId = "series-1",
                Date = new DateTime(2026, 6, 8),
                StartTime = new DateTime(2026, 6, 8, 10, 0, 0),
                EndTime = new DateTime(2026, 6, 8, 11, 0, 0),
                RepeatType = WorkScheduleRepeatTypes.Weekly,
                RepeatUntil = new DateTime(2026, 6, 8),
                ParticipantIds = new List<int> { 1, 2 },
                Participants = "稲生遥希、田中太郎"
            }
        };

        await notificationService.NotifySeriesCreatedAsync(entries, CancellationToken.None);
        await database.Context.SaveChangesAsync();

        var notifications = await database.Context.Notifications
            .OrderBy(notification => notification.UserId)
            .ToListAsync();

        Assert.Equal(2, notifications.Count);
        Assert.All(notifications, notification =>
        {
            Assert.Equal("Info", notification.Type);
            Assert.Equal(10, notification.TargetWorkScheduleEntryId);
            Assert.Contains("社外予定「顧客訪問」を繰り返し登録しました", notification.Message);
            Assert.Contains("繰り返し: 毎週", notification.Message);
            Assert.Contains("件数: 2件", notification.Message);
        });
    }

    [Fact]
    public async Task CreateEntryAsync_CreatesWarningNotifications_WhenExternalAppointmentOverlapsReservation()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.Reservations.Add(new ReservationModel
        {
            UserId = 1,
            Name = "稲生遥希",
            Room = "大会議室",
            NumberOfPeople = 2,
            Type = "社内",
            Purpose = "定例会議",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 10, 0, 0),
            EndTime = new DateTime(2026, 6, 15, 11, 0, 0),
            ParticipantIds = new List<int> { 1, 2 },
            Participants = "稲生遥希、田中太郎",
            RepeatType = "しない"
        });

        await database.Context.SaveChangesAsync();

        var service = CreateWorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 1, 1, 10, 30, 0),
            EndTime = new DateTime(2026, 1, 1, 11, 30, 0),
            ParticipantIds = new List<int> { 2 }
        };

        var result = await service.CreateEntryAsync(
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.NotNull(result.Entry);

        var notifications = await database.Context.Notifications
            .OrderBy(notification => notification.UserId)
            .ToListAsync();

        Assert.Equal(2, notifications.Count);
        Assert.All(notifications, notification =>
        {
            Assert.Equal("Warning", notification.Type);
            Assert.Equal(result.Entry.Id, notification.TargetWorkScheduleEntryId);
            Assert.Contains("注意: 参加者の予定重複があります", notification.Message);
            Assert.Contains("会議室予約「定例会議」", notification.Message);
        });
    }

    [Fact]
    public async Task UpdateEntryAsync_NotifiesRetainedAddedAndRemovedParticipants()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = CreateWorkScheduleEntryService(database.Context);

        var createResult = await service.CreateEntryAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 6, 15),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 }
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.NotNull(createResult.Entry);

        database.Context.Notifications.RemoveRange(database.Context.Notifications);
        await database.Context.SaveChangesAsync();

        var updateResult = await service.UpdateEntryAsync(
            createResult.Entry.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問 更新",
                Date = new DateTime(2026, 6, 15),
                StartTime = new DateTime(2026, 1, 1, 13, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 14, 0, 0),
                ParticipantIds = new List<int> { 3 }
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(updateResult.ErrorMessage);
        Assert.NotNull(updateResult.Entry);

        var notifications = await database.Context.Notifications
            .OrderBy(notification => notification.UserId)
            .ToListAsync();

        Assert.Equal(3, notifications.Count);

        var retainedUserNotification = Assert.Single(notifications, notification => notification.UserId == 1);
        Assert.Equal("Info", retainedUserNotification.Type);
        Assert.Contains("社外予定「顧客訪問 更新」が更新されました", retainedUserNotification.Message);
        Assert.Contains("対象者に 佐藤花子 が追加されました", retainedUserNotification.Message);
        Assert.Contains("対象者から 田中太郎 が削除されました", retainedUserNotification.Message);

        var removedUserNotification = Assert.Single(notifications, notification => notification.UserId == 2);
        Assert.Equal("Info", removedUserNotification.Type);
        Assert.Contains("対象者からあなたが削除されました", removedUserNotification.Message);

        var addedUserNotification = Assert.Single(notifications, notification => notification.UserId == 3);
        Assert.Equal("Info", addedUserNotification.Type);
        Assert.Contains("対象者にあなたが追加されました", addedUserNotification.Message);
    }

    [Fact]
    public async Task DeleteEntryAsync_CreatesDeletedNotificationsForParticipants()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = CreateWorkScheduleEntryService(database.Context);

        var createResult = await service.CreateEntryAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.WorkFromHome,
                Title = "在宅",
                Date = new DateTime(2026, 6, 16),
                ParticipantIds = new List<int> { 2 }
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.NotNull(createResult.Entry);

        database.Context.Notifications.RemoveRange(database.Context.Notifications);
        await database.Context.SaveChangesAsync();

        var deleteResult = await service.DeleteEntryAsync(
            createResult.Entry.Id,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(deleteResult.ErrorMessage);

        var notifications = await database.Context.Notifications
            .OrderBy(notification => notification.UserId)
            .ToListAsync();

        Assert.Equal(2, notifications.Count);
        Assert.All(notifications, notification =>
        {
            Assert.Equal("Info", notification.Type);
            Assert.Equal(createResult.Entry.Id, notification.TargetWorkScheduleEntryId);
            Assert.Contains("在宅予定「在宅」が削除されました", notification.Message);
        });
    }

    [Fact]
    public async Task ReservationNotificationService_AddsWarning_WhenReservationOverlapsExternalAppointment()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.WorkScheduleEntries.Add(new WorkScheduleEntry
        {
            CreatedByUserId = 1,
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 10, 30, 0),
            EndTime = new DateTime(2026, 6, 15, 11, 30, 0),
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 2 },
            Participants = "田中太郎",
            CreatedAt = new DateTime(2026, 6, 1, 9, 0, 0),
            UpdatedAt = new DateTime(2026, 6, 1, 9, 0, 0)
        });

        var reservation = new ReservationModel
        {
            UserId = 1,
            Name = "稲生遥希",
            Room = "大会議室",
            NumberOfPeople = 2,
            Type = "社内",
            Purpose = "定例会議",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 10, 0, 0),
            EndTime = new DateTime(2026, 6, 15, 11, 0, 0),
            ParticipantIds = new List<int> { 1, 2 },
            Participants = "稲生遥希、田中太郎",
            RepeatType = "しない"
        };

        database.Context.Reservations.Add(reservation);
        await database.Context.SaveChangesAsync();

        var notificationService = new ReservationNotificationService(
            database.Context,
            new ReservationSeriesQueryService(database.Context),
            new WorkScheduleParticipantConflictService(database.Context));

        await notificationService.NotifyParticipantsForCreatedReservationAsync(reservation);
        await database.Context.SaveChangesAsync();

        var notification = Assert.Single(await database.Context.Notifications.ToListAsync());

        Assert.Equal(2, notification.UserId);
        Assert.Equal("Warning", notification.Type);
        Assert.Equal(reservation.Id, notification.TargetReservationId);
        Assert.Null(notification.TargetWorkScheduleEntryId);
        Assert.Contains("注意: 参加者の予定重複があります", notification.Message);
        Assert.Contains("社外予定「顧客訪問」", notification.Message);
    }

    private static WorkScheduleEntryService CreateWorkScheduleEntryService(AppDbContext context)
    {
        var conflictService = new WorkScheduleParticipantConflictService(context);
        var notificationService = new WorkScheduleNotificationService(context, conflictService);

        return new WorkScheduleEntryService(context, notificationService);
    }

    private static async Task SeedUsersAsync(AppDbContext context)
    {
        context.Users.AddRange(
            new UserModel
            {
                Id = 1,
                Name = "稲生遥希",
                Email = "haruki@example.com"
            },
            new UserModel
            {
                Id = 2,
                Name = "田中太郎",
                Email = "tanaka@example.com"
            },
            new UserModel
            {
                Id = 3,
                Name = "佐藤花子",
                Email = "sato@example.com"
            });

        await context.SaveChangesAsync();
    }

    private static TestDatabase CreateTestDatabase()
    {
        var connection = new SqliteConnection("Data Source=:memory:");
        connection.Open();

        var options = new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(connection)
            .Options;

        var context = new AppDbContext(options);
        context.Database.EnsureCreated();

        return new TestDatabase(connection, context);
    }

    private sealed class TestDatabase : IDisposable
    {
        private readonly SqliteConnection _connection;

        public TestDatabase(
            SqliteConnection connection,
            AppDbContext context)
        {
            _connection = connection;
            Context = context;
        }

        public AppDbContext Context { get; }

        public void Dispose()
        {
            Context.Dispose();
            _connection.Dispose();
        }
    }
}

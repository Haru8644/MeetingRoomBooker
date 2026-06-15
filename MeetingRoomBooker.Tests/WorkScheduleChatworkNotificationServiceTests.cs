using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace MeetingRoomBooker.Tests;

public sealed class WorkScheduleChatworkNotificationServiceTests
{
    [Fact]
    public async Task SendCreatedAsync_SendsDirectMessagesAndCreatesDeliveryLogs()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var userWithoutDirectRoom = await database.Context.Users
            .SingleAsync(user => user.Id == 3);

        userWithoutDirectRoom.ChatworkDirectRoomId = null;
        await database.Context.SaveChangesAsync();

        var chatworkClient = new RecordingChatworkClient();
        var service = CreateService(database.Context, chatworkClient);

        var entry = new WorkScheduleEntryModel
        {
            Id = 10,
            CreatedByUserId = 1,
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 10, 0, 0),
            EndTime = new DateTime(2026, 6, 15, 11, 0, 0),
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 1, 2, 3 },
            Participants = "稲生遥希、田中太郎、佐藤花子"
        };

        await service.SendCreatedAsync(entry, CancellationToken.None);

        Assert.Equal(2, chatworkClient.SentMessages.Count);
        Assert.Contains(chatworkClient.SentMessages, message => message.RoomId == "room-1");
        Assert.Contains(chatworkClient.SentMessages, message => message.RoomId == "room-2");
        Assert.DoesNotContain(chatworkClient.SentMessages, message => message.RoomId == "room-3");

        Assert.All(chatworkClient.SentMessages, message =>
        {
            Assert.Contains("社外予定が登録されました", message.Message);
            Assert.Contains("内容: 顧客訪問", message.Message);
            Assert.Contains("時間: 10:00〜11:00", message.Message);
        });

        var logs = await database.Context.ChatworkDeliveryLogs
            .OrderBy(log => log.TargetUserId)
            .ToListAsync();

        Assert.Equal(3, logs.Count);

        Assert.All(logs, log =>
        {
            Assert.Equal(0, log.ReservationId);
            Assert.Equal(entry.Id, log.WorkScheduleEntryId);
            Assert.Equal("WorkScheduleCreated", log.DeliveryType);
            Assert.Contains("work-schedule:10", log.DeliveryKey);
        });

        Assert.Equal("Succeeded", logs[0].Status);
        Assert.Equal("Succeeded", logs[1].Status);
        Assert.Equal("Skipped", logs[2].Status);
        Assert.Contains("ChatworkDirectRoomId", logs[2].ErrorMessage);
    }

    [Fact]
    public async Task SendCreatedAsync_IncludesConflictWarning_WhenExternalAppointmentOverlapsReservation()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.Reservations.Add(new ReservationModel
        {
            Id = 100,
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

        var chatworkClient = new RecordingChatworkClient();
        var service = CreateService(database.Context, chatworkClient);

        var entry = new WorkScheduleEntryModel
        {
            Id = 20,
            CreatedByUserId = 1,
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 10, 30, 0),
            EndTime = new DateTime(2026, 6, 15, 11, 30, 0),
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 2 },
            Participants = "田中太郎"
        };

        await service.SendCreatedAsync(entry, CancellationToken.None);

        var sentMessage = Assert.Single(chatworkClient.SentMessages);

        Assert.Equal("room-2", sentMessage.RoomId);
        Assert.Contains("注意: 参加者の予定重複があります", sentMessage.Message);
        Assert.Contains("会議室予約「定例会議」", sentMessage.Message);
        Assert.Contains("田中太郎", sentMessage.Message);

        var log = Assert.Single(await database.Context.ChatworkDeliveryLogs.ToListAsync());
        Assert.Equal(entry.Id, log.WorkScheduleEntryId);
        Assert.Equal("Succeeded", log.Status);
    }

    [Fact]
    public async Task SendUpdatedAsync_SendsDifferentMessagesToRetainedAddedAndRemovedParticipants()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var chatworkClient = new RecordingChatworkClient();
        var service = CreateService(database.Context, chatworkClient);

        var previousEntry = new WorkScheduleEntryModel
        {
            Id = 30,
            CreatedByUserId = 1,
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 10, 0, 0),
            EndTime = new DateTime(2026, 6, 15, 11, 0, 0),
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 1, 2 },
            Participants = "稲生遥希、田中太郎"
        };

        var currentEntry = new WorkScheduleEntryModel
        {
            Id = 30,
            CreatedByUserId = 1,
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問 更新",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 6, 15, 13, 0, 0),
            EndTime = new DateTime(2026, 6, 15, 14, 0, 0),
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 1, 3 },
            Participants = "稲生遥希、佐藤花子"
        };

        await service.SendUpdatedAsync(previousEntry, currentEntry, CancellationToken.None);

        Assert.Equal(3, chatworkClient.SentMessages.Count);

        var retainedMessage = Assert.Single(chatworkClient.SentMessages, message => message.RoomId == "room-1");
        Assert.Contains("社外予定が変更されました", retainedMessage.Message);
        Assert.Contains("内容が「顧客訪問」から「顧客訪問 更新」に変更されました", retainedMessage.Message);
        Assert.Contains("対象者に 佐藤花子 が追加されました", retainedMessage.Message);
        Assert.Contains("対象者から 田中太郎 が削除されました", retainedMessage.Message);

        var removedMessage = Assert.Single(chatworkClient.SentMessages, message => message.RoomId == "room-2");
        Assert.Contains("社外予定の対象者から削除されました", removedMessage.Message);
        Assert.Contains("内容: 顧客訪問", removedMessage.Message);

        var addedMessage = Assert.Single(chatworkClient.SentMessages, message => message.RoomId == "room-3");
        Assert.Contains("社外予定の対象者に追加されました", addedMessage.Message);
        Assert.Contains("内容: 顧客訪問 更新", addedMessage.Message);

        var logs = await database.Context.ChatworkDeliveryLogs
            .OrderBy(log => log.TargetUserId)
            .ToListAsync();

        Assert.Equal(3, logs.Count);
        Assert.All(logs, log =>
        {
            Assert.Equal(currentEntry.Id, log.WorkScheduleEntryId);
            Assert.Equal("WorkScheduleUpdated", log.DeliveryType);
            Assert.Equal("Succeeded", log.Status);
        });
    }

    [Fact]
    public async Task SendDeletedAsync_SendsDeletedMessagesAndCreatesDeliveryLogs()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var chatworkClient = new RecordingChatworkClient();
        var service = CreateService(database.Context, chatworkClient);

        var entry = new WorkScheduleEntryModel
        {
            Id = 40,
            CreatedByUserId = 1,
            Type = WorkScheduleEntryType.Leave,
            Title = "休暇",
            Date = new DateTime(2026, 6, 16),
            LeavePeriod = LeavePeriod.FullDay,
            ParticipantIds = new List<int> { 1, 2 },
            Participants = "稲生遥希、田中太郎"
        };

        await service.SendDeletedAsync(entry, CancellationToken.None);

        Assert.Equal(2, chatworkClient.SentMessages.Count);

        Assert.All(chatworkClient.SentMessages, message =>
        {
            Assert.Contains("休暇予定が削除されました", message.Message);
            Assert.Contains("内容: 休暇", message.Message);
            Assert.Contains("休暇区分: 休み", message.Message);
        });

        var logs = await database.Context.ChatworkDeliveryLogs
            .OrderBy(log => log.TargetUserId)
            .ToListAsync();

        Assert.Equal(2, logs.Count);
        Assert.All(logs, log =>
        {
            Assert.Equal(entry.Id, log.WorkScheduleEntryId);
            Assert.Equal("WorkScheduleDeleted", log.DeliveryType);
            Assert.Equal("Succeeded", log.Status);
        });
    }

    private static WorkScheduleChatworkNotificationService CreateService(
        AppDbContext context,
        RecordingChatworkClient chatworkClient)
    {
        return new WorkScheduleChatworkNotificationService(
            context,
            chatworkClient,
            new WorkScheduleParticipantConflictService(context),
            NullLogger<WorkScheduleChatworkNotificationService>.Instance);
    }

    private static async Task SeedUsersAsync(AppDbContext context)
    {
        context.Users.AddRange(
            new UserModel
            {
                Id = 1,
                Name = "稲生遥希",
                Email = "haruki@example.com",
                ChatworkDirectRoomId = "room-1"
            },
            new UserModel
            {
                Id = 2,
                Name = "田中太郎",
                Email = "tanaka@example.com",
                ChatworkDirectRoomId = "room-2"
            },
            new UserModel
            {
                Id = 3,
                Name = "佐藤花子",
                Email = "sato@example.com",
                ChatworkDirectRoomId = "room-3"
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

    private sealed class RecordingChatworkClient : IChatworkClient
    {
        public List<SentMessage> SentMessages { get; } = new();

        public Task SendMessageAsync(
            string message,
            CancellationToken cancellationToken = default)
        {
            SentMessages.Add(new SentMessage(null, message));
            return Task.CompletedTask;
        }

        public Task SendMessageAsync(
            string roomId,
            string message,
            CancellationToken cancellationToken = default)
        {
            SentMessages.Add(new SentMessage(roomId, message));
            return Task.CompletedTask;
        }
    }

    private sealed record SentMessage(
        string? RoomId,
        string Message);

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

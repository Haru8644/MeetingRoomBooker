using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace MeetingRoomBooker.Tests;

public sealed class ReservationChatworkNotificationServiceTests
{
    [Fact]
    public async Task SendReservationCreatedAsync_IncludesExternalAppointmentConflictWarning()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.WorkScheduleEntries.Add(new WorkScheduleEntry
        {
            Id = 100,
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

        var reservation = CreateReservation();
        database.Context.Reservations.Add(reservation);
        await database.Context.SaveChangesAsync();

        var chatworkClient = new RecordingChatworkClient();
        var service = CreateService(database.Context, chatworkClient);

        await service.SendReservationCreatedAsync(reservation, CancellationToken.None);

        Assert.Equal(2, chatworkClient.SentMessages.Count);

        Assert.All(chatworkClient.SentMessages, sentMessage =>
        {
            Assert.Contains("会議予約を受け付けました", sentMessage.Message);
            Assert.Contains("注意: 参加者の予定が社外予定と重複しています", sentMessage.Message);
            Assert.Contains("田中太郎: 社外予定「顧客訪問」", sentMessage.Message);
            Assert.Contains("10:30〜11:30", sentMessage.Message);
        });

        var logs = await database.Context.ChatworkDeliveryLogs
            .OrderBy(log => log.TargetUserId)
            .ToListAsync();

        Assert.Equal(2, logs.Count);
        Assert.All(logs, log =>
        {
            Assert.Equal(reservation.Id, log.ReservationId);
            Assert.Null(log.WorkScheduleEntryId);
            Assert.Equal("ReservationCreated", log.DeliveryType);
            Assert.Equal("Succeeded", log.Status);
        });
    }

    [Fact]
    public async Task SendReservationUpdatedAsync_SendsWarning_WhenOnlyExternalAppointmentConflictExists()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.WorkScheduleEntries.Add(new WorkScheduleEntry
        {
            Id = 101,
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

        var previousReservation = CreateReservation();
        var currentReservation = CreateReservation();

        database.Context.Reservations.Add(currentReservation);
        await database.Context.SaveChangesAsync();

        var chatworkClient = new RecordingChatworkClient();
        var service = CreateService(database.Context, chatworkClient);

        await service.SendReservationUpdatedAsync(
            previousReservation,
            currentReservation,
            CancellationToken.None);

        Assert.Equal(2, chatworkClient.SentMessages.Count);

        Assert.All(chatworkClient.SentMessages, sentMessage =>
        {
            Assert.Contains("会議予約が変更されました", sentMessage.Message);
            Assert.Contains("注意: 参加者の予定が社外予定と重複しています", sentMessage.Message);
            Assert.Contains("田中太郎: 社外予定「顧客訪問」", sentMessage.Message);
        });

        var logs = await database.Context.ChatworkDeliveryLogs
            .OrderBy(log => log.TargetUserId)
            .ToListAsync();

        Assert.Equal(2, logs.Count);
        Assert.All(logs, log =>
        {
            Assert.Equal(currentReservation.Id, log.ReservationId);
            Assert.Null(log.WorkScheduleEntryId);
            Assert.Equal("ReservationUpdated", log.DeliveryType);
            Assert.Equal("Succeeded", log.Status);
        });
    }

    private static ReservationModel CreateReservation()
    {
        return new ReservationModel
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
            RepeatType = "しない",
            IsInternal = true
        };
    }

    private static ReservationChatworkNotificationService CreateService(
        AppDbContext context,
        RecordingChatworkClient chatworkClient)
    {
        return new ReservationChatworkNotificationService(
            context,
            chatworkClient,
            new FixedChatworkRoomResolver(),
            new WorkScheduleParticipantConflictService(context),
            NullLogger<ReservationChatworkNotificationService>.Instance);
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

    private sealed class FixedChatworkRoomResolver : IChatworkRoomResolver
    {
        public string ResolveFacilityRoomId(ReservationModel reservation)
        {
            return "facility-room";
        }

        public string ResolveStakeholderRoomId()
        {
            return "stakeholder-room";
        }

        public string? ResolveReceptionRoomId(ReservationModel reservation)
        {
            return null;
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

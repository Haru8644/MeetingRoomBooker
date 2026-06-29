using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Options;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Tests;

public sealed class ChatworkReminderWorkerTests
{
    [Fact]
    public async Task ProcessUpcomingRemindersAsync_SendsWorkScheduleReminderOnlyForUpcomingExternalAppointments()
    {
        using var database = CreateTestDatabase();
        var now = new DateTime(2026, 6, 15, 9, 50, 0);

        database.Context.WorkScheduleEntries.AddRange(
            CreateWorkScheduleEntry(
                id: 1,
                type: WorkScheduleEntryType.ExternalAppointment,
                title: "対象の社外予定",
                startTime: now.AddMinutes(5),
                endTime: now.AddMinutes(35)),
            CreateWorkScheduleEntry(
                id: 2,
                type: WorkScheduleEntryType.ExternalAppointment,
                title: "開始済みの社外予定",
                startTime: now,
                endTime: now.AddMinutes(30)),
            CreateWorkScheduleEntry(
                id: 3,
                type: WorkScheduleEntryType.ExternalAppointment,
                title: "10分より後の社外予定",
                startTime: now.AddMinutes(11),
                endTime: now.AddMinutes(40)),
            CreateWorkScheduleEntry(
                id: 4,
                type: WorkScheduleEntryType.WorkFromHome,
                title: "在宅",
                startTime: now.AddMinutes(5),
                endTime: now.AddMinutes(35)),
            CreateWorkScheduleEntry(
                id: 5,
                type: WorkScheduleEntryType.ExternalAppointment,
                title: "終了時刻なし",
                startTime: now.AddMinutes(5),
                endTime: null));

        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var reservationService = new RecordingReservationChatworkNotificationService();
        var workScheduleService = new RecordingWorkScheduleChatworkNotificationService();
        using var provider = CreateServiceProvider(database.Context, reservationService, workScheduleService);
        var worker = CreateWorker(provider);

        await worker.ProcessUpcomingRemindersAsync(now, CancellationToken.None);

        Assert.Empty(reservationService.Reminders);

        var reminder = Assert.Single(workScheduleService.Reminders);
        Assert.Equal(1, reminder.Id);
        Assert.Equal("対象の社外予定", reminder.Title);
    }

    [Fact]
    public async Task ProcessUpcomingRemindersAsync_StillProcessesUpcomingReservationReminders()
    {
        using var database = CreateTestDatabase();
        var now = new DateTime(2026, 6, 15, 9, 50, 0);

        database.Context.Reservations.Add(new ReservationModel
        {
            Id = 10,
            UserId = 1,
            Name = "稲生遥希",
            Room = "会議室A",
            NumberOfPeople = 2,
            Type = "社内",
            Purpose = "定例会議",
            Date = now.Date,
            StartTime = now.AddMinutes(5),
            EndTime = now.AddMinutes(35),
            ParticipantIds = new List<int> { 1 },
            Participants = "稲生遥希",
            RepeatType = "しない"
        });

        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var reservationService = new RecordingReservationChatworkNotificationService();
        var workScheduleService = new RecordingWorkScheduleChatworkNotificationService();
        using var provider = CreateServiceProvider(database.Context, reservationService, workScheduleService);
        var worker = CreateWorker(provider);

        await worker.ProcessUpcomingRemindersAsync(now, CancellationToken.None);

        var reminder = Assert.Single(reservationService.Reminders);
        Assert.Equal(10, reminder.Id);
        Assert.Empty(workScheduleService.Reminders);
    }

    private static ChatworkReminderWorker CreateWorker(ServiceProvider provider)
    {
        return new ChatworkReminderWorker(
            provider.GetRequiredService<IServiceScopeFactory>(),
            Options.Create(new ChatworkOptions { Enabled = true }),
            NullLogger<ChatworkReminderWorker>.Instance);
    }

    private static ServiceProvider CreateServiceProvider(
        AppDbContext context,
        IReservationChatworkNotificationService reservationService,
        IWorkScheduleChatworkNotificationService workScheduleService)
    {
        var services = new ServiceCollection();
        services.AddSingleton(context);
        services.AddSingleton(reservationService);
        services.AddSingleton(workScheduleService);

        return services.BuildServiceProvider();
    }

    private static WorkScheduleEntry CreateWorkScheduleEntry(
        int id,
        WorkScheduleEntryType type,
        string title,
        DateTime? startTime,
        DateTime? endTime)
    {
        return new WorkScheduleEntry
        {
            Id = id,
            CreatedByUserId = 1,
            Type = type,
            Title = title,
            Date = (startTime ?? new DateTime(2026, 6, 15)).Date,
            StartTime = startTime,
            EndTime = endTime,
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 1 },
            Participants = "稲生遥希",
            CreatedAt = new DateTime(2026, 6, 1, 9, 0, 0),
            UpdatedAt = new DateTime(2026, 6, 1, 9, 0, 0)
        };
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

    private sealed class RecordingReservationChatworkNotificationService : IReservationChatworkNotificationService
    {
        public List<ReservationModel> Reminders { get; } = new();

        public Task SendReservationCreatedAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationCreatedAsync(
            ReservationModel reservation,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationUpdatedAsync(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesUpdatedAsync(
            ReservationModel representativePreviousReservation,
            ReservationModel representativeCurrentReservation,
            int updatedCount,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationCanceledAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesCanceledAsync(
            ReservationModel representativeReservation,
            int canceledCount,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationReminderAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            Reminders.Add(reservation);
            return Task.CompletedTask;
        }
    }

    private sealed class RecordingWorkScheduleChatworkNotificationService : IWorkScheduleChatworkNotificationService
    {
        public List<WorkScheduleEntryModel> Reminders { get; } = new();

        public Task SendCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendSeriesCreatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendSeriesDeletedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            string scope,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReminderAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            Reminders.Add(entry);
            return Task.CompletedTask;
        }
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

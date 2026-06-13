using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Tests;

public sealed class WorkScheduleEntryServiceTests
{
    [Fact]
    public async Task CreateEntryAsync_CreatesExternalAppointment()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
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

        Assert.Equal(WorkScheduleEntryType.ExternalAppointment, result.Entry.Type);
        Assert.Equal("顧客訪問", result.Entry.Title);
        Assert.Equal(new DateTime(2026, 6, 15), result.Entry.Date);
        Assert.Equal(new DateTime(2026, 6, 15, 10, 0, 0), result.Entry.StartTime);
        Assert.Equal(new DateTime(2026, 6, 15, 11, 30, 0), result.Entry.EndTime);
        Assert.Equal(LeavePeriod.None, result.Entry.LeavePeriod);
        Assert.Equal(new List<int> { 1, 2 }, result.Entry.ParticipantIds);
        Assert.Equal("稲生遥希、田中太郎", result.Entry.Participants);
    }

    [Fact]
    public async Task CreateEntryAsync_ReturnsBadRequest_WhenExternalAppointmentTimeIsInvalid()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.ExternalAppointment,
            Title = "顧客訪問",
            Date = new DateTime(2026, 6, 15),
            StartTime = new DateTime(2026, 1, 1, 11, 0, 0),
            EndTime = new DateTime(2026, 1, 1, 10, 0, 0),
            ParticipantIds = new List<int> { 1 }
        };

        var result = await service.CreateEntryAsync(
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("終了時刻は開始時刻より後にしてください。", result.ErrorMessage);
        Assert.Null(result.Entry);
    }

    [Fact]
    public async Task CreateEntryAsync_CreatesWorkFromHomeWithDefaultTitle()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.WorkFromHome,
            Date = new DateTime(2026, 6, 16),
            ParticipantIds = new List<int> { 2 }
        };

        var result = await service.CreateEntryAsync(
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.NotNull(result.Entry);

        Assert.Equal(WorkScheduleEntryType.WorkFromHome, result.Entry.Type);
        Assert.Equal("在宅", result.Entry.Title);
        Assert.Equal(new DateTime(2026, 6, 16), result.Entry.Date);
        Assert.Null(result.Entry.StartTime);
        Assert.Null(result.Entry.EndTime);
        Assert.Equal(LeavePeriod.None, result.Entry.LeavePeriod);
        Assert.Equal(new List<int> { 1, 2 }, result.Entry.ParticipantIds);
    }

    [Fact]
    public async Task CreateEntryAsync_CreatesLeaveWithLeavePeriod()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.Leave,
            Date = new DateTime(2026, 6, 17),
            LeavePeriod = LeavePeriod.Morning,
            ParticipantIds = new List<int> { 1 }
        };

        var result = await service.CreateEntryAsync(
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.NotNull(result.Entry);

        Assert.Equal(WorkScheduleEntryType.Leave, result.Entry.Type);
        Assert.Equal("休暇", result.Entry.Title);
        Assert.Equal(new DateTime(2026, 6, 17), result.Entry.Date);
        Assert.Equal(LeavePeriod.Morning, result.Entry.LeavePeriod);
        Assert.Null(result.Entry.StartTime);
        Assert.Null(result.Entry.EndTime);
    }

    [Fact]
    public async Task CreateEntryAsync_ReturnsBadRequest_WhenLeavePeriodIsNone()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var request = new CreateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.Leave,
            Date = new DateTime(2026, 6, 17),
            LeavePeriod = LeavePeriod.None,
            ParticipantIds = new List<int> { 1 }
        };

        var result = await service.CreateEntryAsync(
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("休暇区分を選択してください。", result.ErrorMessage);
        Assert.Null(result.Entry);
    }

    [Fact]
    public async Task GetEntriesAsync_ReturnsEntriesWithinDateRange()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.WorkScheduleEntries.AddRange(
            CreateEntry(
                type: WorkScheduleEntryType.ExternalAppointment,
                title: "対象期間前",
                date: new DateTime(2026, 6, 9),
                createdByUserId: 1),
            CreateEntry(
                type: WorkScheduleEntryType.WorkFromHome,
                title: "在宅",
                date: new DateTime(2026, 6, 10),
                createdByUserId: 1),
            CreateEntry(
                type: WorkScheduleEntryType.Leave,
                title: "休暇",
                date: new DateTime(2026, 6, 20),
                createdByUserId: 1),
            CreateEntry(
                type: WorkScheduleEntryType.ExternalAppointment,
                title: "対象期間後",
                date: new DateTime(2026, 6, 21),
                createdByUserId: 1));

        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var entries = await service.GetEntriesAsync(
            from: new DateTime(2026, 6, 10),
            to: new DateTime(2026, 6, 20),
            CancellationToken.None);

        Assert.Equal(2, entries.Count);
        Assert.Equal("在宅", entries[0].Title);
        Assert.Equal("休暇", entries[1].Title);
    }

    [Fact]
    public async Task UpdateEntryAsync_AllowsOwnerToUpdateEntry()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.WorkFromHome,
            title: "在宅",
            date: new DateTime(2026, 6, 18),
            createdByUserId: 1);

        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var request = new UpdateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.Leave,
            Title = "午後休",
            Date = new DateTime(2026, 6, 18),
            LeavePeriod = LeavePeriod.Afternoon,
            ParticipantIds = new List<int> { 1 }
        };

        var result = await service.UpdateEntryAsync(
            entry.Id,
            request,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);
        Assert.NotNull(result.Entry);

        Assert.Equal(WorkScheduleEntryType.Leave, result.Entry.Type);
        Assert.Equal("午後休", result.Entry.Title);
        Assert.Equal(LeavePeriod.Afternoon, result.Entry.LeavePeriod);
    }

    [Fact]
    public async Task UpdateEntryAsync_ReturnsForbidden_WhenUserIsNotOwnerOrAdmin()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.WorkFromHome,
            title: "在宅",
            date: new DateTime(2026, 6, 18),
            createdByUserId: 1);

        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var request = new UpdateWorkScheduleEntryRequest
        {
            Type = WorkScheduleEntryType.WorkFromHome,
            Title = "在宅更新",
            Date = new DateTime(2026, 6, 18),
            ParticipantIds = new List<int> { 2 }
        };

        var result = await service.UpdateEntryAsync(
            entry.Id,
            request,
            currentUserId: 2,
            isAdmin: false,
            CancellationToken.None);

        Assert.True(result.Forbidden);
        Assert.Null(result.Entry);
    }

    [Fact]
    public async Task DeleteEntryAsync_AllowsAdminToDeleteAnyEntry()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.WorkFromHome,
            title: "在宅",
            date: new DateTime(2026, 6, 18),
            createdByUserId: 1);

        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.DeleteEntryAsync(
            entry.Id,
            currentUserId: 2,
            isAdmin: true,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);
        Assert.Empty(database.Context.WorkScheduleEntries);
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
            });

        await context.SaveChangesAsync();
    }

    private static WorkScheduleEntry CreateEntry(
        WorkScheduleEntryType type,
        string title,
        DateTime date,
        int createdByUserId)
    {
        return new WorkScheduleEntry
        {
            CreatedByUserId = createdByUserId,
            Type = type,
            Title = title,
            Date = date,
            StartTime = type == WorkScheduleEntryType.ExternalAppointment
                ? date.Date.AddHours(10)
                : null,
            EndTime = type == WorkScheduleEntryType.ExternalAppointment
                ? date.Date.AddHours(11)
                : null,
            LeavePeriod = type == WorkScheduleEntryType.Leave
                ? LeavePeriod.FullDay
                : LeavePeriod.None,
            ParticipantIds = new List<int> { createdByUserId },
            Participants = "テストユーザー",
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

using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.RoomConflictRecords;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace MeetingRoomBooker.Tests;

public sealed class RoomConflictDetectionServiceTests
{
    [Fact]
    public async Task DetectUnresolvedOverlapsAsync_CreatesDetectedRecord_WhenOverlapStartIsReached()
    {
        using var database = CreateTestDatabase();

        database.Context.Reservations.AddRange(
            CreateReservation(
                id: 1,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 0, 0),
                endTime: new DateTime(2026, 6, 1, 11, 0, 0)),
            CreateReservation(
                id: 2,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 30, 0),
                endTime: new DateTime(2026, 6, 1, 11, 30, 0)));

        await database.Context.SaveChangesAsync();

        var service = CreateService(database.Context);

        var createdCount = await service.DetectUnresolvedOverlapsAsync(
            now: new DateTime(2026, 6, 1, 10, 31, 0),
            lookbackWindow: TimeSpan.FromMinutes(15),
            cancellationToken: CancellationToken.None);

        Assert.Equal(1, createdCount);

        var record = await database.Context.RoomConflictRecords.SingleAsync();

        Assert.Equal(ConflictRecordType.UnresolvedReservationOverlap, record.Type);
        Assert.Equal(ConflictStatus.Detected, record.Status);
        Assert.Equal(new DateTime(2026, 6, 1, 10, 30, 0), record.OccurredAt);
        Assert.Equal("大会議室", record.RoomName);
        Assert.Equal(1, record.ReservationIdA);
        Assert.Equal(2, record.ReservationIdB);
        Assert.NotNull(record.DetectionKey);
    }

    [Fact]
    public async Task DetectUnresolvedOverlapsAsync_DoesNotCreateRecord_WhenOverlapStartHasNotReached()
    {
        using var database = CreateTestDatabase();

        database.Context.Reservations.AddRange(
            CreateReservation(
                id: 1,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 0, 0),
                endTime: new DateTime(2026, 6, 1, 11, 0, 0)),
            CreateReservation(
                id: 2,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 30, 0),
                endTime: new DateTime(2026, 6, 1, 11, 30, 0)));

        await database.Context.SaveChangesAsync();

        var service = CreateService(database.Context);

        var createdCount = await service.DetectUnresolvedOverlapsAsync(
            now: new DateTime(2026, 6, 1, 10, 29, 0),
            lookbackWindow: TimeSpan.FromMinutes(15),
            cancellationToken: CancellationToken.None);

        Assert.Equal(0, createdCount);
        Assert.Empty(database.Context.RoomConflictRecords);
    }

    [Fact]
    public async Task DetectUnresolvedOverlapsAsync_DoesNotCreateDuplicateRecord()
    {
        using var database = CreateTestDatabase();

        database.Context.Reservations.AddRange(
            CreateReservation(
                id: 1,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 0, 0),
                endTime: new DateTime(2026, 6, 1, 11, 0, 0)),
            CreateReservation(
                id: 2,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 30, 0),
                endTime: new DateTime(2026, 6, 1, 11, 30, 0)));

        await database.Context.SaveChangesAsync();

        var service = CreateService(database.Context);

        var now = new DateTime(2026, 6, 1, 10, 31, 0);

        var firstCount = await service.DetectUnresolvedOverlapsAsync(
            now,
            TimeSpan.FromMinutes(15),
            CancellationToken.None);

        var secondCount = await service.DetectUnresolvedOverlapsAsync(
            now.AddMinutes(1),
            TimeSpan.FromMinutes(15),
            CancellationToken.None);

        Assert.Equal(1, firstCount);
        Assert.Equal(0, secondCount);
        Assert.Equal(1, await database.Context.RoomConflictRecords.CountAsync());
    }

    [Fact]
    public async Task DetectUnresolvedOverlapsAsync_DoesNotCreateRecord_WhenRoomsAreDifferent()
    {
        using var database = CreateTestDatabase();

        database.Context.Reservations.AddRange(
            CreateReservation(
                id: 1,
                room: "大会議室",
                startTime: new DateTime(2026, 6, 1, 10, 0, 0),
                endTime: new DateTime(2026, 6, 1, 11, 0, 0)),
            CreateReservation(
                id: 2,
                room: "小会議室",
                startTime: new DateTime(2026, 6, 1, 10, 30, 0),
                endTime: new DateTime(2026, 6, 1, 11, 30, 0)));

        await database.Context.SaveChangesAsync();

        var service = CreateService(database.Context);

        var createdCount = await service.DetectUnresolvedOverlapsAsync(
            now: new DateTime(2026, 6, 1, 10, 31, 0),
            lookbackWindow: TimeSpan.FromMinutes(15),
            cancellationToken: CancellationToken.None);

        Assert.Equal(0, createdCount);
        Assert.Empty(database.Context.RoomConflictRecords);
    }

    private static RoomConflictDetectionService CreateService(AppDbContext context)
    {
        return new RoomConflictDetectionService(
            context,
            NullLogger<RoomConflictDetectionService>.Instance);
    }

    private static ReservationModel CreateReservation(
        int id,
        string room,
        DateTime startTime,
        DateTime endTime)
    {
        return new ReservationModel
        {
            Id = id,
            UserId = 1,
            Name = "テストユーザー",
            Room = room,
            Date = startTime.Date,
            StartTime = startTime,
            EndTime = endTime,
            NumberOfPeople = 2,
            Purpose = "テスト予約"
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

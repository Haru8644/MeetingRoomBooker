using MeetingRoomBooker.Api.Contracts.RoomConflictRecords;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.RoomConflictRecords;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace MeetingRoomBooker.Tests;

public sealed class RoomConflictRecordServiceTests
{
    [Fact]
    public async Task CreateManualRecordAsync_CreatesActualRoomCollision()
    {
        using var database = CreateTestDatabase();

        var service = new RoomConflictRecordService(database.Context);

        var request = new CreateRoomConflictRecordRequest
        {
            OccurredAt = new DateTime(2026, 6, 1, 10, 0, 0),
            RoomName = "大会議室",
            Impact = ConflictImpact.Medium,
            Cause = ConflictCause.LastMinuteChange,
            Description = "会議開始時に別の会議と重なった。",
            Resolution = "小会議室へ移動した。"
        };

        var result = await service.CreateManualRecordAsync(
            request,
            currentUserId: 1,
            isAdmin: false);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.NotFound);
        Assert.False(result.Forbidden);
        Assert.NotNull(result.Record);

        Assert.Equal(ConflictRecordType.ActualRoomCollision, result.Record.Type);
        Assert.Equal(ConflictStatus.Confirmed, result.Record.Status);
        Assert.Equal("大会議室", result.Record.RoomName);
        Assert.Equal(1, result.Record.ReportedByUserId);
        Assert.True(result.Record.CanEdit);
    }

    [Fact]
    public async Task CreateManualRecordAsync_ReturnsBadRequest_WhenRoomNameIsBlank()
    {
        using var database = CreateTestDatabase();

        var service = new RoomConflictRecordService(database.Context);

        var request = new CreateRoomConflictRecordRequest
        {
            OccurredAt = new DateTime(2026, 6, 1, 10, 0, 0),
            RoomName = " ",
            Impact = ConflictImpact.Low,
            Cause = ConflictCause.Unknown
        };

        var result = await service.CreateManualRecordAsync(
            request,
            currentUserId: 1,
            isAdmin: false);

        Assert.Equal("RoomName is required.", result.ErrorMessage);
        Assert.Null(result.Record);
    }

    [Fact]
    public async Task CreateManualRecordAsync_ReturnsBadRequest_WhenStatusIsDetected()
    {
        using var database = CreateTestDatabase();

        var service = new RoomConflictRecordService(database.Context);

        var request = new CreateRoomConflictRecordRequest
        {
            OccurredAt = new DateTime(2026, 6, 1, 10, 0, 0),
            RoomName = "大会議室",
            Impact = ConflictImpact.Medium,
            Cause = ConflictCause.Unknown,
            Status = ConflictStatus.Detected
        };

        var result = await service.CreateManualRecordAsync(
            request,
            currentUserId: 1,
            isAdmin: false);

        Assert.Equal(
            "Manual conflict reports cannot be created with Detected status.",
            result.ErrorMessage);
        Assert.Null(result.Record);
    }

    [Fact]
    public async Task UpdateRecordAsync_AllowsReporterToUpdateOwnRecord()
    {
        using var database = CreateTestDatabase();

        var record = CreateRecord(
            reportedByUserId: 1,
            status: ConflictStatus.Confirmed);

        database.Context.RoomConflictRecords.Add(record);
        await database.Context.SaveChangesAsync();

        var service = new RoomConflictRecordService(database.Context);

        var request = new UpdateRoomConflictRecordRequest
        {
            Status = ConflictStatus.Resolved,
            Impact = ConflictImpact.Low,
            Cause = ConflictCause.InputMistake,
            Description = "入力ミスによる衝突だった。",
            Resolution = "次回から登録内容を確認する。"
        };

        var result = await service.UpdateRecordAsync(
            record.Id,
            request,
            currentUserId: 1,
            isAdmin: false);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);
        Assert.NotNull(result.Record);

        Assert.Equal(ConflictStatus.Resolved, result.Record.Status);
        Assert.Equal(ConflictImpact.Low, result.Record.Impact);
        Assert.Equal(ConflictCause.InputMistake, result.Record.Cause);
        Assert.True(result.Record.CanEdit);
    }

    [Fact]
    public async Task UpdateRecordAsync_ReturnsForbidden_WhenUserIsNotReporterOrAdmin()
    {
        using var database = CreateTestDatabase();

        var record = CreateRecord(
            reportedByUserId: 1,
            status: ConflictStatus.Confirmed);

        database.Context.RoomConflictRecords.Add(record);
        await database.Context.SaveChangesAsync();

        var service = new RoomConflictRecordService(database.Context);

        var request = new UpdateRoomConflictRecordRequest
        {
            Status = ConflictStatus.Resolved,
            Impact = ConflictImpact.Low,
            Cause = ConflictCause.Other,
            Description = "他人が更新しようとした。",
            Resolution = "更新不可。"
        };

        var result = await service.UpdateRecordAsync(
            record.Id,
            request,
            currentUserId: 2,
            isAdmin: false);

        Assert.True(result.Forbidden);
        Assert.Null(result.Record);
    }

    [Fact]
    public async Task UpdateRecordAsync_AllowsAdminToUpdateAnyRecord()
    {
        using var database = CreateTestDatabase();

        var record = CreateRecord(
            reportedByUserId: 1,
            status: ConflictStatus.Confirmed);

        database.Context.RoomConflictRecords.Add(record);
        await database.Context.SaveChangesAsync();

        var service = new RoomConflictRecordService(database.Context);

        var request = new UpdateRoomConflictRecordRequest
        {
            Status = ConflictStatus.FalseAlarm,
            Impact = ConflictImpact.Low,
            Cause = ConflictCause.Other,
            Description = "実際には衝突していなかった。",
            Resolution = "誤検知として分類した。"
        };

        var result = await service.UpdateRecordAsync(
            record.Id,
            request,
            currentUserId: 99,
            isAdmin: true);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);
        Assert.NotNull(result.Record);

        Assert.Equal(ConflictStatus.FalseAlarm, result.Record.Status);
        Assert.True(result.Record.CanEdit);
    }

    [Fact]
    public async Task GetSummaryAsync_CountsCurrentMonthRecords()
    {
        using var database = CreateTestDatabase();

        database.Context.RoomConflictRecords.AddRange(
            CreateRecord(
                type: ConflictRecordType.UnresolvedReservationOverlap,
                status: ConflictStatus.Detected,
                impact: ConflictImpact.Medium,
                occurredAt: new DateTime(2026, 6, 1, 10, 0, 0)),
            CreateRecord(
                type: ConflictRecordType.ActualRoomCollision,
                status: ConflictStatus.Confirmed,
                impact: ConflictImpact.High,
                occurredAt: new DateTime(2026, 6, 2, 11, 0, 0)),
            CreateRecord(
                type: ConflictRecordType.ActualRoomCollision,
                status: ConflictStatus.Confirmed,
                impact: ConflictImpact.High,
                occurredAt: new DateTime(2026, 5, 31, 11, 0, 0)));

        await database.Context.SaveChangesAsync();

        var service = new RoomConflictRecordService(database.Context);

        var summary = await service.GetSummaryAsync(
            new DateTime(2026, 6, 15, 12, 0, 0));

        Assert.Equal(1, summary.UnresolvedOverlapsThisMonth);
        Assert.Equal(1, summary.ConfirmedCollisionsThisMonth);
        Assert.Equal(1, summary.HighImpactConflictsThisMonth);
        Assert.Equal(1, summary.OpenDetectedRecords);
    }

    private static RoomConflictRecord CreateRecord(
        int? reportedByUserId = 1,
        ConflictRecordType type = ConflictRecordType.ActualRoomCollision,
        ConflictStatus status = ConflictStatus.Confirmed,
        ConflictImpact impact = ConflictImpact.Medium,
        ConflictCause cause = ConflictCause.Unknown,
        DateTime? occurredAt = null)
    {
        return new RoomConflictRecord
        {
            Type = type,
            Status = status,
            OccurredAt = occurredAt ?? new DateTime(2026, 6, 1, 10, 0, 0),
            RoomName = "大会議室",
            Impact = impact,
            Cause = cause,
            Description = "テスト用の衝突記録。",
            Resolution = string.Empty,
            ReportedByUserId = reportedByUserId,
            CreatedAt = new DateTime(2026, 6, 1, 9, 0, 0)
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

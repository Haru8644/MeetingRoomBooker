using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.Chatwork;
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
    public async Task CreateEntrySeriesAsync_WhenDailyExternalAppointmentRequested_CreatesWeekdayEntriesWithSameSeriesId()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var notificationService = new RecordingWorkScheduleNotificationService();
        var chatworkNotificationService = new RecordingWorkScheduleChatworkNotificationService();
        var service = new WorkScheduleEntryService(
            database.Context,
            notificationService,
            chatworkNotificationService);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 6, 5),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = WorkScheduleRepeatTypes.Daily,
                RepeatUntil = new DateTime(2026, 6, 9)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.Equal(3, result.Entries.Count);
        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 5),
            new(2026, 6, 8),
            new(2026, 6, 9)
        }, result.Entries.Select(entry => entry.Date).ToList());

        var seriesId = Assert.Single(result.Entries.Select(entry => entry.SeriesId).Distinct());
        Assert.False(string.IsNullOrWhiteSpace(seriesId));
        Assert.All(result.Entries, entry =>
        {
            Assert.Equal(WorkScheduleRepeatTypes.Daily, entry.RepeatType);
            Assert.Equal(new DateTime(2026, 6, 9), entry.RepeatUntil);
            Assert.Equal(seriesId, entry.SeriesId);
            Assert.Equal(entry.Date.Date.AddHours(10), entry.StartTime);
            Assert.Equal(entry.Date.Date.AddHours(11), entry.EndTime);
        });

        Assert.Equal(3, await database.Context.WorkScheduleEntries.CountAsync());
        Assert.Equal(1, notificationService.SeriesCreatedCallCount);
        Assert.Equal(0, notificationService.CreatedCallCount);
        Assert.Equal(1, chatworkNotificationService.SeriesCreatedCallCount);
        Assert.Equal(0, chatworkNotificationService.CreatedCallCount);
    }

    [Fact]
    public async Task CreateEntrySeriesAsync_WhenWeeklyExternalAppointmentRequested_CreatesSevenDayStepEntries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "週次訪問",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = WorkScheduleRepeatTypes.Weekly,
                RepeatUntil = new DateTime(2026, 6, 15)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 1),
            new(2026, 6, 8),
            new(2026, 6, 15)
        }, result.Entries.Select(entry => entry.Date).ToList());
    }

    [Theory]
    [InlineData("隔週")]
    [InlineData("毎月")]
    [InlineData("invalid")]
    public async Task CreateEntrySeriesAsync_ReturnsBadRequest_WhenRepeatTypeIsUnsupported(string repeatType)
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = repeatType,
                RepeatUntil = new DateTime(2026, 6, 15)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.NotNull(result.ErrorMessage);
        Assert.Empty(result.Entries);
        Assert.Empty(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task CreateEntrySeriesAsync_ReturnsBadRequest_WhenRepeatUntilIsMissing()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = WorkScheduleRepeatTypes.Daily
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し終了日を入力してください。", result.ErrorMessage);
        Assert.Empty(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task CreateEntrySeriesAsync_ReturnsBadRequest_WhenRepeatUntilIsBeforeStartDate()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 6, 2),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = WorkScheduleRepeatTypes.Daily,
                RepeatUntil = new DateTime(2026, 6, 1)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し終了日は開始日以降にしてください。", result.ErrorMessage);
        Assert.Empty(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task CreateEntrySeriesAsync_ReturnsBadRequest_WhenGeneratedEntryCountExceedsLimit()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 1, 1),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = WorkScheduleRepeatTypes.Daily,
                RepeatUntil = new DateTime(2026, 4, 30)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し作成は最大60件までです。", result.ErrorMessage);
        Assert.Empty(database.Context.WorkScheduleEntries);
    }

    [Theory]
    [InlineData(WorkScheduleEntryType.WorkFromHome)]
    [InlineData(WorkScheduleEntryType.Leave)]
    public async Task CreateEntrySeriesAsync_ReturnsBadRequest_WhenNonExternalAppointmentUsesRecurrence(
        WorkScheduleEntryType type)
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = type,
                Date = new DateTime(2026, 6, 1),
                LeavePeriod = type == WorkScheduleEntryType.Leave ? LeavePeriod.FullDay : LeavePeriod.None,
                ParticipantIds = new List<int> { 1 },
                RepeatType = WorkScheduleRepeatTypes.Weekly,
                RepeatUntil = new DateTime(2026, 6, 15)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し作成は社外予定のみ指定できます。", result.ErrorMessage);
        Assert.Empty(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task CreateEntrySeriesAsync_WhenSummaryNotificationFails_KeepsCreatedEntries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var service = new WorkScheduleEntryService(
            database.Context,
            new ThrowingWorkScheduleNotificationService(),
            new ThrowingWorkScheduleChatworkNotificationService());

        var result = await service.CreateEntrySeriesAsync(
            new CreateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "顧客訪問",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 10, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 11, 0, 0),
                ParticipantIds = new List<int> { 2 },
                RepeatType = WorkScheduleRepeatTypes.Weekly,
                RepeatUntil = new DateTime(2026, 6, 8)
            },
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.Equal(2, result.Entries.Count);
        Assert.Equal(2, await database.Context.WorkScheduleEntries.CountAsync());
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

    [Fact]
    public async Task UpdateEntrySeriesAsync_WhenScopeSingle_UpdatesOnlySelectedEntry()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);
        var third = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 3), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second, third);
        await database.Context.SaveChangesAsync();

        var notificationService = new RecordingWorkScheduleNotificationService();
        var chatworkNotificationService = new RecordingWorkScheduleChatworkNotificationService();
        var service = new WorkScheduleEntryService(
            database.Context,
            notificationService,
            chatworkNotificationService);

        var result = await service.UpdateEntrySeriesAsync(
            second.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "更新後の顧客訪問",
                Date = new DateTime(2026, 6, 20),
                StartTime = new DateTime(2026, 1, 1, 14, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 15, 0, 0),
                ParticipantIds = new List<int> { 1, 2 }
            },
            WorkScheduleSeriesScopes.Single,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);
        Assert.NotNull(result.Entry);

        var entries = await database.Context.WorkScheduleEntries
            .AsNoTracking()
            .ToDictionaryAsync(entry => entry.Id);

        Assert.Equal("顧客訪問", entries[first.Id].Title);
        Assert.Equal(new DateTime(2026, 6, 1), entries[first.Id].Date.Date);
        Assert.Equal("更新後の顧客訪問", entries[second.Id].Title);
        Assert.Equal(new DateTime(2026, 6, 20), entries[second.Id].Date.Date);
        Assert.Equal(new DateTime(2026, 6, 20, 14, 0, 0), entries[second.Id].StartTime);
        Assert.Equal(new DateTime(2026, 6, 20, 15, 0, 0), entries[second.Id].EndTime);
        Assert.Equal("顧客訪問", entries[third.Id].Title);
        Assert.Equal(new DateTime(2026, 6, 3), entries[third.Id].Date.Date);
        Assert.Equal(1, notificationService.UpdatedCallCount);
        Assert.Equal(0, notificationService.SeriesUpdatedCallCount);
        Assert.Equal(1, chatworkNotificationService.UpdatedCallCount);
        Assert.Equal(0, chatworkNotificationService.SeriesUpdatedCallCount);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_WhenScopeFollowing_UpdatesSelectedAndFutureEntriesKeepingDates()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);
        var third = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 3), createdByUserId: 1);
        var otherSeries = CreateSeriesEntry("series-2", new DateTime(2026, 6, 3), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second, third, otherSeries);
        await database.Context.SaveChangesAsync();

        var notificationService = new RecordingWorkScheduleNotificationService();
        var chatworkNotificationService = new RecordingWorkScheduleChatworkNotificationService();
        var service = new WorkScheduleEntryService(
            database.Context,
            notificationService,
            chatworkNotificationService);

        var result = await service.UpdateEntrySeriesAsync(
            second.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "更新後の顧客訪問",
                Date = new DateTime(2026, 6, 20),
                StartTime = new DateTime(2026, 1, 1, 14, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 15, 0, 0),
                ParticipantIds = new List<int> { 1, 2 }
            },
            WorkScheduleSeriesScopes.Following,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);

        var entries = await database.Context.WorkScheduleEntries
            .AsNoTracking()
            .OrderBy(entry => entry.Id)
            .ToDictionaryAsync(entry => entry.Id);

        Assert.Equal("顧客訪問", entries[first.Id].Title);
        Assert.Equal(new DateTime(2026, 6, 1), entries[first.Id].Date.Date);
        Assert.Equal(new DateTime(2026, 6, 1, 10, 0, 0), entries[first.Id].StartTime);

        Assert.Equal("更新後の顧客訪問", entries[second.Id].Title);
        Assert.Equal(new DateTime(2026, 6, 2), entries[second.Id].Date.Date);
        Assert.Equal(new DateTime(2026, 6, 2, 14, 0, 0), entries[second.Id].StartTime);
        Assert.Equal(new DateTime(2026, 6, 2, 15, 0, 0), entries[second.Id].EndTime);
        Assert.Equal(new List<int> { 1, 2 }, entries[second.Id].ParticipantIds);
        Assert.Equal("稲生遥希、田中太郎", entries[second.Id].Participants);

        Assert.Equal("更新後の顧客訪問", entries[third.Id].Title);
        Assert.Equal(new DateTime(2026, 6, 3), entries[third.Id].Date.Date);
        Assert.Equal(new DateTime(2026, 6, 3, 14, 0, 0), entries[third.Id].StartTime);
        Assert.Equal(new DateTime(2026, 6, 3, 15, 0, 0), entries[third.Id].EndTime);

        Assert.Equal("顧客訪問", entries[otherSeries.Id].Title);
        Assert.Equal(0, notificationService.UpdatedCallCount);
        Assert.Equal(1, notificationService.SeriesUpdatedCallCount);
        Assert.Equal(2, notificationService.LastSeriesUpdatedEntriesCount);
        Assert.Equal(WorkScheduleSeriesScopes.Following, notificationService.LastSeriesUpdatedScope);
        Assert.Equal(0, chatworkNotificationService.UpdatedCallCount);
        Assert.Equal(1, chatworkNotificationService.SeriesUpdatedCallCount);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_WhenScopeAll_UpdatesAllEntriesInSameSeries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);
        var third = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 3), createdByUserId: 1);
        var otherSeries = CreateSeriesEntry("series-2", new DateTime(2026, 6, 3), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second, third, otherSeries);
        await database.Context.SaveChangesAsync();

        var notificationService = new RecordingWorkScheduleNotificationService();
        var service = new WorkScheduleEntryService(database.Context, notificationService);

        var result = await service.UpdateEntrySeriesAsync(
            second.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "全件更新後",
                Date = new DateTime(2026, 6, 20),
                StartTime = new DateTime(2026, 1, 1, 9, 30, 0),
                EndTime = new DateTime(2026, 1, 1, 10, 30, 0),
                ParticipantIds = new List<int> { 1 }
            },
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);

        var entries = await database.Context.WorkScheduleEntries
            .AsNoTracking()
            .OrderBy(entry => entry.Id)
            .ToDictionaryAsync(entry => entry.Id);

        Assert.All(new[] { first.Id, second.Id, third.Id }, id =>
        {
            Assert.Equal("全件更新後", entries[id].Title);
            Assert.Equal(entries[id].Date.Date.AddHours(9).AddMinutes(30), entries[id].StartTime);
            Assert.Equal(entries[id].Date.Date.AddHours(10).AddMinutes(30), entries[id].EndTime);
        });

        Assert.Equal("顧客訪問", entries[otherSeries.Id].Title);
        Assert.Equal(1, notificationService.SeriesUpdatedCallCount);
        Assert.Equal(3, notificationService.LastSeriesUpdatedEntriesCount);
        Assert.Equal(WorkScheduleSeriesScopes.All, notificationService.LastSeriesUpdatedScope);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_ReturnsBadRequest_WhenFollowingRequestedForEntryWithoutSeriesId()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.ExternalAppointment,
            title: "顧客訪問",
            date: new DateTime(2026, 6, 1),
            createdByUserId: 1);

        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.UpdateEntrySeriesAsync(
            entry.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "更新後",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 14, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 15, 0, 0),
                ParticipantIds = new List<int> { 1 }
            },
            WorkScheduleSeriesScopes.Following,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し社外予定のみまとめて編集できます。", result.ErrorMessage);
        Assert.Single(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_ReturnsBadRequest_WhenFollowingRequestedForWorkFromHome()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.WorkFromHome,
            title: "在宅",
            date: new DateTime(2026, 6, 1),
            createdByUserId: 1);

        entry.SeriesId = "series-1";
        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.UpdateEntrySeriesAsync(
            entry.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.WorkFromHome,
                Title = "在宅更新",
                Date = new DateTime(2026, 6, 1),
                ParticipantIds = new List<int> { 1 }
            },
            WorkScheduleSeriesScopes.Following,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し社外予定のみまとめて編集できます。", result.ErrorMessage);
        Assert.Single(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_ReturnsBadRequest_WhenScopeIsInvalid()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateSeriesEntry("series-1", new DateTime(2026, 6, 1), createdByUserId: 1);
        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.UpdateEntrySeriesAsync(
            entry.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "更新後",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 14, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 15, 0, 0),
                ParticipantIds = new List<int> { 1 }
            },
            "invalid",
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("反映範囲が不正です。", result.ErrorMessage);
        Assert.Single(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_ReturnsForbidden_WhenNonAdminUpdatesSeriesContainingOtherUsersEntry()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 2);

        database.Context.WorkScheduleEntries.AddRange(first, second);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.UpdateEntrySeriesAsync(
            first.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "更新後",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 14, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 15, 0, 0),
                ParticipantIds = new List<int> { 1 }
            },
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.True(result.Forbidden);
        Assert.Equal(2, database.Context.WorkScheduleEntries.Count());
        Assert.All(database.Context.WorkScheduleEntries, entry => Assert.Equal("顧客訪問", entry.Title));
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_AllowsAdminToUpdateOtherUsersSeries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 2);

        database.Context.WorkScheduleEntries.AddRange(first, second);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.UpdateEntrySeriesAsync(
            first.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "管理者更新",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 13, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 14, 0, 0),
                ParticipantIds = new List<int> { 1 }
            },
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
            isAdmin: true,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);

        var titles = await database.Context.WorkScheduleEntries
            .AsNoTracking()
            .Select(entry => entry.Title)
            .ToListAsync();

        Assert.Equal(new[] { "管理者更新", "管理者更新" }, titles);
    }

    [Fact]
    public async Task UpdateEntrySeriesAsync_WhenSummaryNotificationFails_KeepsUpdatedEntries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(
            database.Context,
            new ThrowingWorkScheduleNotificationService(),
            new ThrowingWorkScheduleChatworkNotificationService());

        var result = await service.UpdateEntrySeriesAsync(
            first.Id,
            new UpdateWorkScheduleEntryRequest
            {
                Type = WorkScheduleEntryType.ExternalAppointment,
                Title = "通知失敗後も更新",
                Date = new DateTime(2026, 6, 1),
                StartTime = new DateTime(2026, 1, 1, 13, 0, 0),
                EndTime = new DateTime(2026, 1, 1, 14, 0, 0),
                ParticipantIds = new List<int> { 1 }
            },
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);

        var titles = await database.Context.WorkScheduleEntries
            .AsNoTracking()
            .Select(entry => entry.Title)
            .ToListAsync();

        Assert.Equal(new[] { "通知失敗後も更新", "通知失敗後も更新" }, titles);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_WhenScopeSingle_DeletesOnlySelectedEntry()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);
        var third = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 3), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second, third);
        await database.Context.SaveChangesAsync();

        var notificationService = new RecordingWorkScheduleNotificationService();
        var chatworkNotificationService = new RecordingWorkScheduleChatworkNotificationService();
        var service = new WorkScheduleEntryService(
            database.Context,
            notificationService,
            chatworkNotificationService);

        var result = await service.DeleteEntrySeriesAsync(
            second.Id,
            WorkScheduleSeriesScopes.Single,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);

        var remainingIds = await database.Context.WorkScheduleEntries
            .OrderBy(entry => entry.Id)
            .Select(entry => entry.Id)
            .ToArrayAsync();

        Assert.Equal(new[] { first.Id, third.Id }, remainingIds);
        Assert.Equal(1, notificationService.DeletedCallCount);
        Assert.Equal(0, notificationService.SeriesDeletedCallCount);
        Assert.Equal(1, chatworkNotificationService.DeletedCallCount);
        Assert.Equal(0, chatworkNotificationService.SeriesDeletedCallCount);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_WhenScopeFollowing_DeletesSelectedAndFutureEntriesOnly()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);
        var third = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 3), createdByUserId: 1);
        var otherSeries = CreateSeriesEntry("series-2", new DateTime(2026, 6, 3), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second, third, otherSeries);
        await database.Context.SaveChangesAsync();

        var notificationService = new RecordingWorkScheduleNotificationService();
        var chatworkNotificationService = new RecordingWorkScheduleChatworkNotificationService();
        var service = new WorkScheduleEntryService(
            database.Context,
            notificationService,
            chatworkNotificationService);

        var result = await service.DeleteEntrySeriesAsync(
            second.Id,
            WorkScheduleSeriesScopes.Following,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);

        var remainingIds = await database.Context.WorkScheduleEntries
            .OrderBy(entry => entry.Id)
            .Select(entry => entry.Id)
            .ToArrayAsync();

        Assert.Equal(new[] { first.Id, otherSeries.Id }, remainingIds);
        Assert.Equal(0, notificationService.DeletedCallCount);
        Assert.Equal(1, notificationService.SeriesDeletedCallCount);
        Assert.Equal(2, notificationService.LastSeriesDeletedEntriesCount);
        Assert.Equal(WorkScheduleSeriesScopes.Following, notificationService.LastSeriesDeletedScope);
        Assert.Equal(0, chatworkNotificationService.DeletedCallCount);
        Assert.Equal(1, chatworkNotificationService.SeriesDeletedCallCount);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_WhenScopeAll_DeletesAllEntriesInSameSeries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 1);
        var third = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 3), createdByUserId: 1);
        var otherSeries = CreateSeriesEntry("series-2", new DateTime(2026, 6, 3), createdByUserId: 1);

        database.Context.WorkScheduleEntries.AddRange(first, second, third, otherSeries);
        await database.Context.SaveChangesAsync();

        var notificationService = new RecordingWorkScheduleNotificationService();
        var service = new WorkScheduleEntryService(database.Context, notificationService);

        var result = await service.DeleteEntrySeriesAsync(
            second.Id,
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Null(result.ErrorMessage);
        Assert.False(result.Forbidden);

        var remainingEntry = Assert.Single(await database.Context.WorkScheduleEntries.ToListAsync());
        Assert.Equal(otherSeries.Id, remainingEntry.Id);
        Assert.Equal(1, notificationService.SeriesDeletedCallCount);
        Assert.Equal(3, notificationService.LastSeriesDeletedEntriesCount);
        Assert.Equal(WorkScheduleSeriesScopes.All, notificationService.LastSeriesDeletedScope);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_ReturnsBadRequest_WhenFollowingRequestedForEntryWithoutSeriesId()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.ExternalAppointment,
            title: "顧客訪問",
            date: new DateTime(2026, 6, 1),
            createdByUserId: 1);

        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.DeleteEntrySeriesAsync(
            entry.Id,
            WorkScheduleSeriesScopes.Following,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し社外予定のみまとめて削除できます。", result.ErrorMessage);
        Assert.Single(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_ReturnsBadRequest_WhenFollowingRequestedForWorkFromHome()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateEntry(
            type: WorkScheduleEntryType.WorkFromHome,
            title: "在宅",
            date: new DateTime(2026, 6, 1),
            createdByUserId: 1);

        entry.SeriesId = "series-1";
        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.DeleteEntrySeriesAsync(
            entry.Id,
            WorkScheduleSeriesScopes.Following,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("繰り返し社外予定のみまとめて削除できます。", result.ErrorMessage);
        Assert.Single(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_ReturnsBadRequest_WhenScopeIsInvalid()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var entry = CreateSeriesEntry("series-1", new DateTime(2026, 6, 1), createdByUserId: 1);
        database.Context.WorkScheduleEntries.Add(entry);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.DeleteEntrySeriesAsync(
            entry.Id,
            "invalid",
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.Equal("削除範囲が不正です。", result.ErrorMessage);
        Assert.Single(database.Context.WorkScheduleEntries);
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_ReturnsForbidden_WhenNonAdminDeletesSeriesContainingOtherUsersEntry()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 2);

        database.Context.WorkScheduleEntries.AddRange(first, second);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.DeleteEntrySeriesAsync(
            first.Id,
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
            isAdmin: false,
            CancellationToken.None);

        Assert.True(result.Forbidden);
        Assert.Equal(2, database.Context.WorkScheduleEntries.Count());
    }

    [Fact]
    public async Task DeleteEntrySeriesAsync_AllowsAdminToDeleteOtherUsersSeries()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var seriesId = "series-1";
        var first = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 1), createdByUserId: 1);
        var second = CreateSeriesEntry(seriesId, new DateTime(2026, 6, 2), createdByUserId: 2);

        database.Context.WorkScheduleEntries.AddRange(first, second);
        await database.Context.SaveChangesAsync();

        var service = new WorkScheduleEntryService(database.Context);

        var result = await service.DeleteEntrySeriesAsync(
            first.Id,
            WorkScheduleSeriesScopes.All,
            currentUserId: 1,
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

    private static WorkScheduleEntry CreateSeriesEntry(
        string seriesId,
        DateTime date,
        int createdByUserId)
    {
        var entry = CreateEntry(
            WorkScheduleEntryType.ExternalAppointment,
            "顧客訪問",
            date,
            createdByUserId);

        entry.SeriesId = seriesId;
        entry.RepeatType = WorkScheduleRepeatTypes.Daily;
        entry.RepeatUntil = new DateTime(2026, 6, 30);
        entry.ParticipantIds = new List<int> { createdByUserId };
        entry.Participants = createdByUserId == 1 ? "稲生遥希" : "田中太郎";

        return entry;
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

    private sealed class RecordingWorkScheduleNotificationService : IWorkScheduleNotificationService
    {
        public int CreatedCallCount { get; private set; }

        public int SeriesCreatedCallCount { get; private set; }

        public int UpdatedCallCount { get; private set; }

        public int SeriesUpdatedCallCount { get; private set; }

        public int LastSeriesUpdatedEntriesCount { get; private set; }

        public string? LastSeriesUpdatedScope { get; private set; }

        public int DeletedCallCount { get; private set; }

        public int SeriesDeletedCallCount { get; private set; }

        public int LastSeriesDeletedEntriesCount { get; private set; }

        public string? LastSeriesDeletedScope { get; private set; }

        public Task NotifyCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken)
        {
            CreatedCallCount++;
            return Task.CompletedTask;
        }

        public Task NotifySeriesCreatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            CancellationToken cancellationToken)
        {
            SeriesCreatedCallCount++;
            return Task.CompletedTask;
        }

        public Task NotifyUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken)
        {
            UpdatedCallCount++;
            return Task.CompletedTask;
        }

        public Task NotifySeriesUpdatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> previousEntries,
            IReadOnlyList<WorkScheduleEntryModel> currentEntries,
            string scope,
            CancellationToken cancellationToken)
        {
            SeriesUpdatedCallCount++;
            LastSeriesUpdatedEntriesCount = currentEntries.Count;
            LastSeriesUpdatedScope = scope;
            return Task.CompletedTask;
        }

        public Task NotifyDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken)
        {
            DeletedCallCount++;
            return Task.CompletedTask;
        }

        public Task NotifySeriesDeletedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            string scope,
            CancellationToken cancellationToken)
        {
            SeriesDeletedCallCount++;
            LastSeriesDeletedEntriesCount = entries.Count;
            LastSeriesDeletedScope = scope;
            return Task.CompletedTask;
        }
    }

    private sealed class RecordingWorkScheduleChatworkNotificationService : IWorkScheduleChatworkNotificationService
    {
        public int CreatedCallCount { get; private set; }

        public int SeriesCreatedCallCount { get; private set; }

        public int UpdatedCallCount { get; private set; }

        public int SeriesUpdatedCallCount { get; private set; }

        public int DeletedCallCount { get; private set; }

        public int SeriesDeletedCallCount { get; private set; }

        public Task SendCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            CreatedCallCount++;
            return Task.CompletedTask;
        }

        public Task SendSeriesCreatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            CancellationToken cancellationToken = default)
        {
            SeriesCreatedCallCount++;
            return Task.CompletedTask;
        }

        public Task SendUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken = default)
        {
            UpdatedCallCount++;
            return Task.CompletedTask;
        }

        public Task SendSeriesUpdatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> previousEntries,
            IReadOnlyList<WorkScheduleEntryModel> currentEntries,
            string scope,
            CancellationToken cancellationToken = default)
        {
            SeriesUpdatedCallCount++;
            return Task.CompletedTask;
        }

        public Task SendDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            DeletedCallCount++;
            return Task.CompletedTask;
        }

        public Task SendSeriesDeletedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            string scope,
            CancellationToken cancellationToken = default)
        {
            SeriesDeletedCallCount++;
            return Task.CompletedTask;
        }

        public Task SendReminderAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }
    }

    private sealed class ThrowingWorkScheduleNotificationService : IWorkScheduleNotificationService
    {
        public Task NotifyCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken)
        {
            throw new InvalidOperationException("notification failed");
        }

        public Task NotifySeriesCreatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            CancellationToken cancellationToken)
        {
            throw new InvalidOperationException("series notification failed");
        }

        public Task NotifyUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken)
        {
            throw new InvalidOperationException("notification failed");
        }

        public Task NotifySeriesUpdatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> previousEntries,
            IReadOnlyList<WorkScheduleEntryModel> currentEntries,
            string scope,
            CancellationToken cancellationToken)
        {
            throw new InvalidOperationException("series update notification failed");
        }

        public Task NotifyDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken)
        {
            throw new InvalidOperationException("notification failed");
        }

        public Task NotifySeriesDeletedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            string scope,
            CancellationToken cancellationToken)
        {
            throw new InvalidOperationException("series delete notification failed");
        }
    }

    private sealed class ThrowingWorkScheduleChatworkNotificationService : IWorkScheduleChatworkNotificationService
    {
        public Task SendCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("chatwork failed");
        }

        public Task SendSeriesCreatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("series chatwork failed");
        }

        public Task SendUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("chatwork failed");
        }

        public Task SendSeriesUpdatedAsync(
            IReadOnlyList<WorkScheduleEntryModel> previousEntries,
            IReadOnlyList<WorkScheduleEntryModel> currentEntries,
            string scope,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("series update chatwork failed");
        }

        public Task SendDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("chatwork failed");
        }

        public Task SendSeriesDeletedAsync(
            IReadOnlyList<WorkScheduleEntryModel> entries,
            string scope,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("series delete chatwork failed");
        }

        public Task SendReminderAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            throw new InvalidOperationException("chatwork failed");
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

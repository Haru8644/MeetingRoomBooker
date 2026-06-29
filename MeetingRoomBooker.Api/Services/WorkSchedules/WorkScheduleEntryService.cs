using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public sealed class WorkScheduleEntryService : IWorkScheduleEntryService
{
    private const int TitleMaxLength = 100;
    private const int ParticipantsMaxLength = 500;
    private const int MaxRecurringEntryCount = 60;

    private readonly AppDbContext _context;
    private readonly IWorkScheduleNotificationService? _notificationService;
    private readonly IWorkScheduleChatworkNotificationService? _chatworkNotificationService;
    private readonly ILogger<WorkScheduleEntryService>? _logger;

    public WorkScheduleEntryService(AppDbContext context)
        : this(context, null, null, null)
    {
    }

    public WorkScheduleEntryService(
        AppDbContext context,
        IWorkScheduleNotificationService? notificationService)
        : this(context, notificationService, null, null)
    {
    }

    public WorkScheduleEntryService(
        AppDbContext context,
        IWorkScheduleNotificationService? notificationService,
        IWorkScheduleChatworkNotificationService? chatworkNotificationService)
        : this(context, notificationService, chatworkNotificationService, null)
    {
    }

    public WorkScheduleEntryService(
        AppDbContext context,
        IWorkScheduleNotificationService? notificationService,
        IWorkScheduleChatworkNotificationService? chatworkNotificationService,
        ILogger<WorkScheduleEntryService>? logger)
    {
        _context = context;
        _notificationService = notificationService;
        _chatworkNotificationService = chatworkNotificationService;
        _logger = logger;
    }

    public async Task<IReadOnlyList<WorkScheduleEntryModel>> GetEntriesAsync(
        DateTime? from,
        DateTime? to,
        CancellationToken cancellationToken)
    {
        var fromDate = from?.Date ?? DateTime.Today.AddDays(-7);
        var toDate = to?.Date ?? DateTime.Today.AddDays(31);

        if (fromDate > toDate)
        {
            return Array.Empty<WorkScheduleEntryModel>();
        }

        var entries = await _context.WorkScheduleEntries
            .AsNoTracking()
            .Where(entry => entry.Date >= fromDate && entry.Date <= toDate)
            .OrderBy(entry => entry.Date)
            .ThenBy(entry => entry.StartTime ?? entry.Date)
            .ThenBy(entry => entry.Type)
            .ThenBy(entry => entry.Id)
            .ToListAsync(cancellationToken);

        return entries
            .Select(ToModel)
            .ToList();
    }

    public async Task<WorkScheduleEntryModel?> GetEntryAsync(
        int id,
        CancellationToken cancellationToken)
    {
        var entry = await _context.WorkScheduleEntries
            .AsNoTracking()
            .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

        return entry == null ? null : ToModel(entry);
    }

    public async Task<WorkScheduleEntryResult> CreateEntryAsync(
        CreateWorkScheduleEntryRequest request,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken)
    {
        if (request.Type != WorkScheduleEntryType.ExternalAppointment &&
            HasRecurrenceInput(request.RepeatType, request.RepeatUntil))
        {
            return WorkScheduleEntryResult.BadRequest("繰り返し作成は社外予定のみ指定できます。");
        }

        if (!WorkScheduleRecurrence.IsNone(request.RepeatType))
        {
            return WorkScheduleEntryResult.BadRequest("繰り返し作成は社外予定の一括作成APIを使用してください。");
        }

        var normalized = await NormalizeCreateRequestAsync(
            request,
            currentUserId,
            cancellationToken);

        if (normalized.ErrorMessage != null)
        {
            return WorkScheduleEntryResult.BadRequest(normalized.ErrorMessage);
        }

        var now = DateTime.Now;

        var entry = new WorkScheduleEntry
        {
            CreatedByUserId = currentUserId,
            Type = normalized.Type,
            Title = normalized.Title,
            Date = normalized.Date,
            StartTime = normalized.StartTime,
            EndTime = normalized.EndTime,
            LeavePeriod = normalized.LeavePeriod,
            ParticipantIds = normalized.ParticipantIds,
            Participants = normalized.Participants,
            CreatedAt = now,
            UpdatedAt = now
        };

        _context.WorkScheduleEntries.Add(entry);
        await _context.SaveChangesAsync(cancellationToken);

        var response = await GetEntryAsync(entry.Id, cancellationToken);
        if (response != null)
        {
            if (_notificationService != null)
            {
                await _notificationService.NotifyCreatedAsync(response, cancellationToken);
                await _context.SaveChangesAsync(cancellationToken);
            }

            if (_chatworkNotificationService != null)
            {
                await _chatworkNotificationService.SendCreatedAsync(response, cancellationToken);
            }
        }

        return WorkScheduleEntryResult.Success(response);
    }

    public async Task<WorkScheduleEntrySeriesResult> CreateEntrySeriesAsync(
        CreateWorkScheduleEntryRequest request,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken)
    {
        var repeatType = NormalizeRepeatType(request.RepeatType);
        if (WorkScheduleRecurrence.IsNone(repeatType))
        {
            var singleResult = await CreateEntryAsync(
                request,
                currentUserId,
                isAdmin,
                cancellationToken);

            return singleResult.ErrorMessage == null && singleResult.Entry != null
                ? WorkScheduleEntrySeriesResult.Success(new List<WorkScheduleEntryModel> { singleResult.Entry })
                : WorkScheduleEntrySeriesResult.BadRequest(singleResult.ErrorMessage ?? "社外予定の登録に失敗しました。");
        }

        if (request.Type != WorkScheduleEntryType.ExternalAppointment)
        {
            return WorkScheduleEntrySeriesResult.BadRequest("繰り返し作成は社外予定のみ指定できます。");
        }

        if (!WorkScheduleRecurrence.IsSupported(repeatType))
        {
            return WorkScheduleEntrySeriesResult.BadRequest("繰り返しの種類が不正です。");
        }

        if (!request.RepeatUntil.HasValue)
        {
            return WorkScheduleEntrySeriesResult.BadRequest("繰り返し終了日を入力してください。");
        }

        if (request.RepeatUntil.Value.Date < request.Date.Date)
        {
            return WorkScheduleEntrySeriesResult.BadRequest("繰り返し終了日は開始日以降にしてください。");
        }

        var targetDates = WorkScheduleRecurrence.BuildTargetDates(
            request.Date,
            repeatType,
            request.RepeatUntil.Value);

        if (targetDates.Count == 0)
        {
            return WorkScheduleEntrySeriesResult.BadRequest("作成対象の日付がありません。");
        }

        if (targetDates.Count > MaxRecurringEntryCount)
        {
            return WorkScheduleEntrySeriesResult.BadRequest($"繰り返し作成は最大{MaxRecurringEntryCount}件までです。");
        }

        var now = DateTime.Now;
        var seriesId = Guid.NewGuid().ToString("N");
        var entries = new List<WorkScheduleEntry>();

        foreach (var targetDate in targetDates)
        {
            var normalized = await NormalizeCreateRequestAsync(
                new CreateWorkScheduleEntryRequest
                {
                    Type = request.Type,
                    Title = request.Title,
                    Date = targetDate,
                    StartTime = request.StartTime,
                    EndTime = request.EndTime,
                    LeavePeriod = request.LeavePeriod,
                    ParticipantIds = request.ParticipantIds,
                    Participants = request.Participants
                },
                currentUserId,
                cancellationToken);

            if (normalized.ErrorMessage != null)
            {
                return WorkScheduleEntrySeriesResult.BadRequest(normalized.ErrorMessage);
            }

            entries.Add(new WorkScheduleEntry
            {
                CreatedByUserId = currentUserId,
                Type = normalized.Type,
                Title = normalized.Title,
                SeriesId = seriesId,
                Date = normalized.Date,
                StartTime = normalized.StartTime,
                EndTime = normalized.EndTime,
                RepeatType = repeatType,
                RepeatUntil = request.RepeatUntil.Value.Date,
                LeavePeriod = normalized.LeavePeriod,
                ParticipantIds = normalized.ParticipantIds,
                Participants = normalized.Participants,
                CreatedAt = now,
                UpdatedAt = now
            });
        }

        await using var transaction = await _context.Database.BeginTransactionAsync(cancellationToken);

        _context.WorkScheduleEntries.AddRange(entries);
        await _context.SaveChangesAsync(cancellationToken);

        await transaction.CommitAsync(cancellationToken);

        var savedEntries = await _context.WorkScheduleEntries
            .AsNoTracking()
            .Where(entry => entry.SeriesId == seriesId)
            .OrderBy(entry => entry.Date)
            .ThenBy(entry => entry.StartTime ?? entry.Date)
            .ThenBy(entry => entry.Id)
            .ToListAsync(cancellationToken);

        var response = savedEntries
            .Select(ToModel)
            .ToList();

        if (response.Count > 0)
        {
            await TrySendSeriesCreatedNotificationsAsync(response, cancellationToken);
        }

        return WorkScheduleEntrySeriesResult.Success(response);
    }

    public async Task<WorkScheduleEntryResult> UpdateEntryAsync(
        int id,
        UpdateWorkScheduleEntryRequest request,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken)
    {
        if (request.Type != WorkScheduleEntryType.ExternalAppointment &&
            HasRecurrenceInput(request.RepeatType, request.RepeatUntil))
        {
            return WorkScheduleEntryResult.BadRequest("繰り返し作成は社外予定のみ指定できます。");
        }

        if (!WorkScheduleRecurrence.IsNone(request.RepeatType))
        {
            return WorkScheduleEntryResult.BadRequest("繰り返し予定の一括更新は未対応です。");
        }

        var entry = await _context.WorkScheduleEntries
            .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

        if (entry == null)
        {
            return WorkScheduleEntryResult.NotFoundResult();
        }

        if (!CanManage(entry, currentUserId, isAdmin))
        {
            return WorkScheduleEntryResult.ForbiddenResult();
        }

        var normalized = await NormalizeUpdateRequestAsync(
            request,
            entry.CreatedByUserId,
            cancellationToken);

        if (normalized.ErrorMessage != null)
        {
            return WorkScheduleEntryResult.BadRequest(normalized.ErrorMessage);
        }

        var previousEntry = ToModel(entry);

        entry.Type = normalized.Type;
        entry.Title = normalized.Title;
        entry.Date = normalized.Date;
        entry.StartTime = normalized.StartTime;
        entry.EndTime = normalized.EndTime;
        entry.LeavePeriod = normalized.LeavePeriod;
        entry.ParticipantIds = normalized.ParticipantIds;
        entry.Participants = normalized.Participants;
        entry.UpdatedAt = DateTime.Now;

        await _context.SaveChangesAsync(cancellationToken);

        var response = await GetEntryAsync(entry.Id, cancellationToken);
        if (response != null)
        {
            if (_notificationService != null)
            {
                await _notificationService.NotifyUpdatedAsync(previousEntry, response, cancellationToken);
                await _context.SaveChangesAsync(cancellationToken);
            }

            if (_chatworkNotificationService != null)
            {
                await _chatworkNotificationService.SendUpdatedAsync(previousEntry, response, cancellationToken);
            }
        }

        return WorkScheduleEntryResult.Success(response);
    }

    public async Task<WorkScheduleEntryResult> DeleteEntryAsync(
        int id,
        int currentUserId,
        bool isAdmin,
        CancellationToken cancellationToken)
    {
        var entry = await _context.WorkScheduleEntries
            .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

        if (entry == null)
        {
            return WorkScheduleEntryResult.NotFoundResult();
        }

        if (!CanManage(entry, currentUserId, isAdmin))
        {
            return WorkScheduleEntryResult.ForbiddenResult();
        }

        var deletedEntry = ToModel(entry);

        _context.WorkScheduleEntries.Remove(entry);
        await _context.SaveChangesAsync(cancellationToken);

        if (_notificationService != null)
        {
            await _notificationService.NotifyDeletedAsync(deletedEntry, cancellationToken);
            await _context.SaveChangesAsync(cancellationToken);
        }

        if (_chatworkNotificationService != null)
        {
            await _chatworkNotificationService.SendDeletedAsync(deletedEntry, cancellationToken);
        }

        return WorkScheduleEntryResult.Success();
    }

    private async Task TrySendSeriesCreatedNotificationsAsync(
        IReadOnlyList<WorkScheduleEntryModel> entries,
        CancellationToken cancellationToken)
    {
        if (_notificationService != null)
        {
            try
            {
                await _notificationService.NotifySeriesCreatedAsync(entries, cancellationToken);
                await _context.SaveChangesAsync(cancellationToken);
            }
            catch (Exception ex)
            {
                _logger?.LogError(
                    ex,
                    "Failed to create system notifications for work schedule series {SeriesId}.",
                    entries.FirstOrDefault()?.SeriesId);
                _context.ChangeTracker.Clear();
            }
        }

        if (_chatworkNotificationService != null)
        {
            try
            {
                await _chatworkNotificationService.SendSeriesCreatedAsync(entries, cancellationToken);
            }
            catch (Exception ex)
            {
                _logger?.LogError(
                    ex,
                    "Failed to send Chatwork notifications for work schedule series {SeriesId}.",
                    entries.FirstOrDefault()?.SeriesId);
                _context.ChangeTracker.Clear();
            }
        }
    }

    private async Task<NormalizedWorkScheduleEntry> NormalizeCreateRequestAsync(
        CreateWorkScheduleEntryRequest request,
        int currentUserId,
        CancellationToken cancellationToken)
    {
        return await NormalizeRequestAsync(
            request.Type,
            request.Title,
            request.Date,
            request.StartTime,
            request.EndTime,
            request.LeavePeriod,
            request.ParticipantIds,
            request.Participants,
            currentUserId,
            cancellationToken);
    }

    private async Task<NormalizedWorkScheduleEntry> NormalizeUpdateRequestAsync(
        UpdateWorkScheduleEntryRequest request,
        int fallbackUserId,
        CancellationToken cancellationToken)
    {
        return await NormalizeRequestAsync(
            request.Type,
            request.Title,
            request.Date,
            request.StartTime,
            request.EndTime,
            request.LeavePeriod,
            request.ParticipantIds,
            request.Participants,
            fallbackUserId,
            cancellationToken);
    }

    private async Task<NormalizedWorkScheduleEntry> NormalizeRequestAsync(
        WorkScheduleEntryType type,
        string? title,
        DateTime date,
        DateTime? startTime,
        DateTime? endTime,
        LeavePeriod leavePeriod,
        List<int>? participantIds,
        string? participants,
        int fallbackUserId,
        CancellationToken cancellationToken)
    {
        if (!Enum.IsDefined(type))
        {
            return NormalizedWorkScheduleEntry.BadRequest("予定種別が不正です。");
        }

        if (!Enum.IsDefined(leavePeriod))
        {
            return NormalizedWorkScheduleEntry.BadRequest("休暇区分が不正です。");
        }

        if (date == default)
        {
            return NormalizedWorkScheduleEntry.BadRequest("日付を入力してください。");
        }

        var normalizedDate = date.Date;
        var normalizedTitle = NormalizeText(title);
        var normalizedParticipantIds = NormalizeParticipantIds(participantIds, fallbackUserId);
        var normalizedParticipants = NormalizeText(participants);

        var participantValidationError = await ValidateParticipantIdsAsync(
            normalizedParticipantIds,
            cancellationToken);

        if (participantValidationError != null)
        {
            return NormalizedWorkScheduleEntry.BadRequest(participantValidationError);
        }

        if (string.IsNullOrWhiteSpace(normalizedParticipants))
        {
            normalizedParticipants = await BuildParticipantsTextAsync(
                normalizedParticipantIds,
                cancellationToken);
        }

        if (normalizedParticipants.Length > ParticipantsMaxLength)
        {
            return NormalizedWorkScheduleEntry.BadRequest(
                $"参加者は{ParticipantsMaxLength}文字以内で入力してください。");
        }

        if (normalizedTitle.Length > TitleMaxLength)
        {
            return NormalizedWorkScheduleEntry.BadRequest(
                $"内容は{TitleMaxLength}文字以内で入力してください。");
        }

        return type switch
        {
            WorkScheduleEntryType.ExternalAppointment => NormalizeExternalAppointment(
                normalizedDate,
                normalizedTitle,
                startTime,
                endTime,
                normalizedParticipantIds,
                normalizedParticipants),

            WorkScheduleEntryType.WorkFromHome => NormalizeWorkFromHome(
                normalizedDate,
                normalizedTitle,
                normalizedParticipantIds,
                normalizedParticipants),

            WorkScheduleEntryType.Leave => NormalizeLeave(
                normalizedDate,
                normalizedTitle,
                leavePeriod,
                normalizedParticipantIds,
                normalizedParticipants),

            _ => NormalizedWorkScheduleEntry.BadRequest("予定種別が不正です。")
        };
    }

    private static NormalizedWorkScheduleEntry NormalizeExternalAppointment(
        DateTime date,
        string title,
        DateTime? startTime,
        DateTime? endTime,
        List<int> participantIds,
        string participants)
    {
        if (string.IsNullOrWhiteSpace(title))
        {
            return NormalizedWorkScheduleEntry.BadRequest("社外予定の内容を入力してください。");
        }

        if (!startTime.HasValue || !endTime.HasValue)
        {
            return NormalizedWorkScheduleEntry.BadRequest("社外予定の開始時刻と終了時刻を入力してください。");
        }

        var normalizedStartTime = date.Date + startTime.Value.TimeOfDay;
        var normalizedEndTime = date.Date + endTime.Value.TimeOfDay;

        if (normalizedStartTime >= normalizedEndTime)
        {
            return NormalizedWorkScheduleEntry.BadRequest("終了時刻は開始時刻より後にしてください。");
        }

        return NormalizedWorkScheduleEntry.Success(
            WorkScheduleEntryType.ExternalAppointment,
            title,
            date,
            normalizedStartTime,
            normalizedEndTime,
            LeavePeriod.None,
            participantIds,
            participants);
    }

    private static NormalizedWorkScheduleEntry NormalizeWorkFromHome(
        DateTime date,
        string title,
        List<int> participantIds,
        string participants)
    {
        return NormalizedWorkScheduleEntry.Success(
            WorkScheduleEntryType.WorkFromHome,
            string.IsNullOrWhiteSpace(title) ? "在宅" : title,
            date,
            null,
            null,
            LeavePeriod.None,
            participantIds,
            participants);
    }

    private static NormalizedWorkScheduleEntry NormalizeLeave(
        DateTime date,
        string title,
        LeavePeriod leavePeriod,
        List<int> participantIds,
        string participants)
    {
        if (leavePeriod == LeavePeriod.None)
        {
            return NormalizedWorkScheduleEntry.BadRequest("休暇区分を選択してください。");
        }

        return NormalizedWorkScheduleEntry.Success(
            WorkScheduleEntryType.Leave,
            string.IsNullOrWhiteSpace(title) ? "休暇" : title,
            date,
            null,
            null,
            leavePeriod,
            participantIds,
            participants);
    }

    private async Task<string?> ValidateParticipantIdsAsync(
        List<int> participantIds,
        CancellationToken cancellationToken)
    {
        if (participantIds.Count == 0)
        {
            return null;
        }

        var existingUserIds = await _context.Users
            .AsNoTracking()
            .Where(user => participantIds.Contains(user.Id))
            .Select(user => user.Id)
            .ToListAsync(cancellationToken);

        var missingUserIds = participantIds
            .Except(existingUserIds)
            .ToList();

        if (missingUserIds.Count > 0)
        {
            return $"存在しない参加者が含まれています: {string.Join(", ", missingUserIds)}";
        }

        return null;
    }

    private async Task<string> BuildParticipantsTextAsync(
        List<int> participantIds,
        CancellationToken cancellationToken)
    {
        if (participantIds.Count == 0)
        {
            return string.Empty;
        }

        var users = await _context.Users
            .AsNoTracking()
            .Where(user => participantIds.Contains(user.Id))
            .OrderBy(user => user.Id)
            .Select(user => new
            {
                user.Id,
                user.Name
            })
            .ToListAsync(cancellationToken);

        return string.Join(
            "、",
            users
                .OrderBy(user => participantIds.IndexOf(user.Id))
                .Select(user => user.Name));
    }

    private static List<int> NormalizeParticipantIds(
        List<int>? participantIds,
        int fallbackUserId)
    {
        var normalized = (participantIds ?? new List<int>())
            .Where(id => id > 0)
            .Distinct()
            .ToList();

        if (fallbackUserId > 0 && !normalized.Contains(fallbackUserId))
        {
            normalized.Insert(0, fallbackUserId);
        }

        return normalized;
    }

    private static string NormalizeText(string? value)
    {
        return value?.Trim() ?? string.Empty;
    }

    private static string NormalizeRepeatType(string? repeatType)
    {
        return string.IsNullOrWhiteSpace(repeatType)
            ? WorkScheduleRepeatTypes.None
            : repeatType.Trim();
    }

    private static bool HasRecurrenceInput(
        string? repeatType,
        DateTime? repeatUntil)
    {
        return !WorkScheduleRecurrence.IsNone(repeatType)
            || repeatUntil.HasValue;
    }

    private static bool CanManage(
        WorkScheduleEntry entry,
        int currentUserId,
        bool isAdmin)
    {
        return isAdmin || entry.CreatedByUserId == currentUserId;
    }

    private static WorkScheduleEntryModel ToModel(WorkScheduleEntry entry)
    {
        return new WorkScheduleEntryModel
        {
            Id = entry.Id,
            CreatedByUserId = entry.CreatedByUserId,
            Type = entry.Type,
            Title = entry.Title,
            SeriesId = entry.SeriesId,
            Date = entry.Date,
            StartTime = entry.StartTime,
            EndTime = entry.EndTime,
            RepeatType = entry.RepeatType,
            RepeatUntil = entry.RepeatUntil,
            LeavePeriod = entry.LeavePeriod,
            ParticipantIds = new List<int>(entry.ParticipantIds ?? new List<int>()),
            Participants = entry.Participants,
            CreatedAt = entry.CreatedAt,
            UpdatedAt = entry.UpdatedAt
        };
    }

    private sealed class NormalizedWorkScheduleEntry
    {
        private NormalizedWorkScheduleEntry()
        {
        }

        public WorkScheduleEntryType Type { get; private init; }

        public string Title { get; private init; } = string.Empty;

        public DateTime Date { get; private init; }

        public DateTime? StartTime { get; private init; }

        public DateTime? EndTime { get; private init; }

        public LeavePeriod LeavePeriod { get; private init; }

        public List<int> ParticipantIds { get; private init; } = new();

        public string Participants { get; private init; } = string.Empty;

        public string? ErrorMessage { get; private init; }

        public static NormalizedWorkScheduleEntry Success(
            WorkScheduleEntryType type,
            string title,
            DateTime date,
            DateTime? startTime,
            DateTime? endTime,
            LeavePeriod leavePeriod,
            List<int> participantIds,
            string participants)
        {
            return new NormalizedWorkScheduleEntry
            {
                Type = type,
                Title = title,
                Date = date,
                StartTime = startTime,
                EndTime = endTime,
                LeavePeriod = leavePeriod,
                ParticipantIds = participantIds,
                Participants = participants
            };
        }

        public static NormalizedWorkScheduleEntry BadRequest(string errorMessage)
        {
            return new NormalizedWorkScheduleEntry
            {
                ErrorMessage = errorMessage
            };
        }
    }
}

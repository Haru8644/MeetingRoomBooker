using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public sealed class WorkScheduleNotificationService : IWorkScheduleNotificationService
{
    private const string InfoNotificationType = "Info";
    private const string WarningNotificationType = "Warning";

    private readonly AppDbContext _context;
    private readonly IWorkScheduleParticipantConflictService _conflictService;

    public WorkScheduleNotificationService(
        AppDbContext context,
        IWorkScheduleParticipantConflictService conflictService)
    {
        _context = context;
        _conflictService = conflictService;
    }

    public async Task NotifyCreatedAsync(
        WorkScheduleEntryModel entry,
        CancellationToken cancellationToken)
    {
        var targetUserIds = GetNotifiableParticipantIds(entry);
        if (targetUserIds.Count == 0)
        {
            return;
        }

        var conflicts = await _conflictService.FindExternalAppointmentConflictsAsync(
            entry,
            cancellationToken);

        var message = BuildCreatedMessage(entry, conflicts);
        var notificationType = conflicts.Count > 0
            ? WarningNotificationType
            : InfoNotificationType;

        foreach (var userId in targetUserIds)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                message,
                entry.Date,
                entry.Id,
                cancellationToken);
        }
    }

    public async Task NotifyUpdatedAsync(
        WorkScheduleEntryModel previousEntry,
        WorkScheduleEntryModel currentEntry,
        CancellationToken cancellationToken)
    {
        var previousParticipantIds = GetNotifiableParticipantIds(previousEntry);
        var currentParticipantIds = GetNotifiableParticipantIds(currentEntry);
        var retainedParticipantIds = currentParticipantIds.Intersect(previousParticipantIds).ToList();
        var addedParticipantIds = currentParticipantIds.Except(previousParticipantIds).ToList();
        var removedParticipantIds = previousParticipantIds.Except(currentParticipantIds).ToList();

        var conflicts = await _conflictService.FindExternalAppointmentConflictsAsync(
            currentEntry,
            cancellationToken);

        var changeLines = await BuildChangeLinesAsync(
            previousEntry,
            currentEntry,
            addedParticipantIds,
            removedParticipantIds,
            cancellationToken);

        var notificationType = conflicts.Count > 0
            ? WarningNotificationType
            : InfoNotificationType;

        foreach (var userId in addedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                BuildAddedParticipantMessage(currentEntry, conflicts),
                currentEntry.Date,
                currentEntry.Id,
                cancellationToken);
        }

        foreach (var userId in removedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                InfoNotificationType,
                BuildRemovedParticipantMessage(previousEntry),
                previousEntry.Date,
                previousEntry.Id,
                cancellationToken);
        }

        if (changeLines.Count == 0 && conflicts.Count == 0)
        {
            return;
        }

        var updatedMessage = BuildUpdatedMessage(currentEntry, changeLines, conflicts);
        foreach (var userId in retainedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                updatedMessage,
                currentEntry.Date,
                currentEntry.Id,
                cancellationToken);
        }
    }

    public async Task NotifyDeletedAsync(
        WorkScheduleEntryModel entry,
        CancellationToken cancellationToken)
    {
        var targetUserIds = GetNotifiableParticipantIds(entry);
        if (targetUserIds.Count == 0)
        {
            return;
        }

        var message = BuildDeletedMessage(entry);
        foreach (var userId in targetUserIds)
        {
            await UpsertNotificationAsync(
                userId,
                InfoNotificationType,
                message,
                entry.Date,
                entry.Id,
                cancellationToken);
        }
    }

    private async Task<IReadOnlyList<string>> BuildChangeLinesAsync(
        WorkScheduleEntryModel previousEntry,
        WorkScheduleEntryModel currentEntry,
        IReadOnlyCollection<int> addedParticipantIds,
        IReadOnlyCollection<int> removedParticipantIds,
        CancellationToken cancellationToken)
    {
        var changes = new List<string>();

        if (previousEntry.Type != currentEntry.Type)
        {
            changes.Add($"種別が {GetTypeLabel(previousEntry.Type)} から {GetTypeLabel(currentEntry.Type)} に変更されました");
        }

        if (!string.Equals(previousEntry.Title, currentEntry.Title, StringComparison.Ordinal))
        {
            changes.Add($"内容が「{previousEntry.Title}」から「{currentEntry.Title}」に変更されました");
        }

        if (previousEntry.Date.Date != currentEntry.Date.Date)
        {
            changes.Add($"日付が {previousEntry.Date:yyyy/MM/dd} から {currentEntry.Date:yyyy/MM/dd} に変更されました");
        }

        if (previousEntry.StartTime != currentEntry.StartTime || previousEntry.EndTime != currentEntry.EndTime)
        {
            changes.Add($"時間が {GetTimeRangeText(previousEntry)} から {GetTimeRangeText(currentEntry)} に変更されました");
        }

        if (previousEntry.LeavePeriod != currentEntry.LeavePeriod)
        {
            changes.Add($"休暇区分が {GetLeavePeriodLabel(previousEntry.LeavePeriod)} から {GetLeavePeriodLabel(currentEntry.LeavePeriod)} に変更されました");
        }

        var participantNames = await GetUserNamesByIdAsync(
            addedParticipantIds.Concat(removedParticipantIds),
            cancellationToken);

        var addedNames = FormatUserNames(addedParticipantIds, participantNames);
        if (!string.IsNullOrWhiteSpace(addedNames))
        {
            changes.Add($"対象者に {addedNames} が追加されました");
        }

        var removedNames = FormatUserNames(removedParticipantIds, participantNames);
        if (!string.IsNullOrWhiteSpace(removedNames))
        {
            changes.Add($"対象者から {removedNames} が削除されました");
        }

        return changes;
    }

    private async Task<Dictionary<int, string>> GetUserNamesByIdAsync(
        IEnumerable<int> userIds,
        CancellationToken cancellationToken)
    {
        var distinctIds = userIds
            .Where(id => id > 0)
            .Distinct()
            .ToList();

        if (distinctIds.Count == 0)
        {
            return new Dictionary<int, string>();
        }

        return await _context.Users
            .AsNoTracking()
            .Where(user => distinctIds.Contains(user.Id))
            .ToDictionaryAsync(user => user.Id, user => user.Name, cancellationToken);
    }

    private async Task UpsertNotificationAsync(
        int userId,
        string type,
        string message,
        DateTime targetDate,
        int targetWorkScheduleEntryId,
        CancellationToken cancellationToken)
    {
        var candidates = await _context.Notifications
            .Where(notification =>
                notification.UserId == userId &&
                notification.Type == type &&
                notification.TargetWorkScheduleEntryId == targetWorkScheduleEntryId)
            .OrderByDescending(notification => notification.CreatedAt)
            .ToListAsync(cancellationToken);

        var existing = candidates.FirstOrDefault(notification => notification.Message == message);

        if (existing == null)
        {
            _context.Notifications.Add(new NotificationModel
            {
                UserId = userId,
                Type = type,
                Message = message,
                TargetDate = targetDate,
                TargetReservationId = null,
                TargetWorkScheduleEntryId = targetWorkScheduleEntryId,
                CreatedAt = DateTime.Now,
                IsRead = false
            });

            return;
        }

        existing.Message = message;
        existing.TargetDate = targetDate;
        existing.CreatedAt = DateTime.Now;
        existing.IsRead = false;

        if (candidates.Count > 1)
        {
            _context.Notifications.RemoveRange(candidates.Skip(1));
        }
    }

    private static string BuildCreatedMessage(
        WorkScheduleEntryModel entry,
        IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
    {
        return $"{GetTypeLabel(entry.Type)}「{entry.Title}」が登録されました。{BuildEntrySummary(entry)}{BuildConflictText(conflicts)}";
    }

    private static string BuildUpdatedMessage(
        WorkScheduleEntryModel entry,
        IReadOnlyCollection<string> changeLines,
        IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
    {
        var changeText = changeLines.Count == 0
            ? string.Empty
            : $" 変更点: {string.Join(" ", changeLines.Select(change => $"{change}。"))}";

        return $"{GetTypeLabel(entry.Type)}「{entry.Title}」が更新されました。{BuildEntrySummary(entry)}{changeText}{BuildConflictText(conflicts)}";
    }

    private static string BuildAddedParticipantMessage(
        WorkScheduleEntryModel entry,
        IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
    {
        return $"{GetTypeLabel(entry.Type)}「{entry.Title}」の対象者にあなたが追加されました。{BuildEntrySummary(entry)}{BuildConflictText(conflicts)}";
    }

    private static string BuildRemovedParticipantMessage(WorkScheduleEntryModel entry)
    {
        return $"{GetTypeLabel(entry.Type)}「{entry.Title}」の対象者からあなたが削除されました。日付: {entry.Date:yyyy/MM/dd}";
    }

    private static string BuildDeletedMessage(WorkScheduleEntryModel entry)
    {
        return $"{GetTypeLabel(entry.Type)}「{entry.Title}」が削除されました。日付: {entry.Date:yyyy/MM/dd}";
    }

    private static string BuildEntrySummary(WorkScheduleEntryModel entry)
    {
        var items = new List<string>
        {
            $"日付: {entry.Date:yyyy/MM/dd}",
            $"対象者: {GetParticipantText(entry)}"
        };

        if (entry.Type == WorkScheduleEntryType.ExternalAppointment)
        {
            items.Insert(1, $"時間: {GetTimeRangeText(entry)}");
        }

        if (entry.Type == WorkScheduleEntryType.Leave)
        {
            items.Insert(1, $"休暇区分: {GetLeavePeriodLabel(entry.LeavePeriod)}");
        }

        return string.Join(" / ", items);
    }

    private static string BuildConflictText(IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
    {
        if (conflicts.Count == 0)
        {
            return string.Empty;
        }

        var lines = conflicts
            .GroupBy(conflict => new
            {
                conflict.ParticipantUserId,
                conflict.SourceType,
                conflict.SourceId
            })
            .Select(group => group.First())
            .Take(5)
            .Select(conflict =>
                $"{conflict.ParticipantName}: {conflict.SourceType}「{conflict.SourceTitle}」 {conflict.Date:yyyy/MM/dd} {GetTimeRangeText(conflict.StartTime, conflict.EndTime)}")
            .ToList();

        var suffix = conflicts.Count > 5
            ? $" ほか{conflicts.Count - 5}件"
            : string.Empty;

        return $" 注意: 参加者の予定重複があります。{string.Join(" ", lines)}{suffix}";
    }

    private static List<int> GetNotifiableParticipantIds(WorkScheduleEntryModel entry)
    {
        return (entry.ParticipantIds ?? new List<int>())
            .Where(id => id > 0)
            .Distinct()
            .ToList();
    }

    private static string GetParticipantText(WorkScheduleEntryModel entry)
    {
        return string.IsNullOrWhiteSpace(entry.Participants)
            ? "未設定"
            : entry.Participants;
    }

    private static string FormatUserNames(
        IEnumerable<int> userIds,
        IReadOnlyDictionary<int, string> userNamesById)
    {
        var names = userIds
            .Distinct()
            .Select(id => userNamesById.TryGetValue(id, out var name) ? name : $"ユーザー{id}")
            .Where(name => !string.IsNullOrWhiteSpace(name))
            .ToList();

        return string.Join("、", names);
    }

    private static string GetTypeLabel(WorkScheduleEntryType type)
    {
        return type switch
        {
            WorkScheduleEntryType.ExternalAppointment => "社外予定",
            WorkScheduleEntryType.WorkFromHome => "在宅予定",
            WorkScheduleEntryType.Leave => "休暇予定",
            _ => "勤務予定"
        };
    }

    private static string GetLeavePeriodLabel(LeavePeriod period)
    {
        return period switch
        {
            LeavePeriod.Morning => "午前休",
            LeavePeriod.Afternoon => "午後休",
            LeavePeriod.FullDay => "休み",
            _ => "なし"
        };
    }

    private static string GetTimeRangeText(WorkScheduleEntryModel entry)
    {
        return GetTimeRangeText(entry.StartTime, entry.EndTime);
    }

    private static string GetTimeRangeText(DateTime? startTime, DateTime? endTime)
    {
        return startTime.HasValue && endTime.HasValue
            ? $"{startTime.Value:HH:mm}〜{endTime.Value:HH:mm}"
            : "終日";
    }
}

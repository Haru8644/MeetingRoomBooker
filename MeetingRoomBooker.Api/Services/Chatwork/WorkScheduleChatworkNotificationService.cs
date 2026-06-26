using System.Text;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class WorkScheduleChatworkNotificationService : IWorkScheduleChatworkNotificationService
    {
        private readonly AppDbContext _context;
        private readonly IChatworkClient _chatworkClient;
        private readonly IWorkScheduleParticipantConflictService _conflictService;
        private readonly ILogger<WorkScheduleChatworkNotificationService> _logger;

        public WorkScheduleChatworkNotificationService(
            AppDbContext context,
            IChatworkClient chatworkClient,
            IWorkScheduleParticipantConflictService conflictService,
            ILogger<WorkScheduleChatworkNotificationService> logger)
        {
            _context = context;
            _chatworkClient = chatworkClient;
            _conflictService = conflictService;
            _logger = logger;
        }

        public async Task SendCreatedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            var targetUserIds = GetParticipantIds(entry);
            var usersById = await GetUsersByIdAsync(targetUserIds, cancellationToken);
            var targetUsers = GetUsers(targetUserIds, usersById);

            if (targetUsers.Count == 0)
            {
                return;
            }

            var conflicts = await _conflictService.FindExternalAppointmentConflictsAsync(
                entry,
                cancellationToken);

            var message = BuildCreatedMessage(entry, conflicts);

            await SendDirectNotificationsAsync(
                entry,
                targetUsers,
                ChatworkDeliveryTypes.WorkScheduleCreated,
                user => ChatworkDeliveryKeys.WorkScheduleCreated(entry.Id, user.Id),
                message,
                cancellationToken);
        }

        public async Task SendUpdatedAsync(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            CancellationToken cancellationToken = default)
        {
            var previousParticipantIds = GetParticipantIds(previousEntry);
            var currentParticipantIds = GetParticipantIds(currentEntry);

            var retainedParticipantIds = currentParticipantIds.Intersect(previousParticipantIds).ToList();
            var addedParticipantIds = currentParticipantIds.Except(previousParticipantIds).ToList();
            var removedParticipantIds = previousParticipantIds.Except(currentParticipantIds).ToList();

            var allTargetUserIds = retainedParticipantIds
                .Union(addedParticipantIds)
                .Union(removedParticipantIds)
                .ToList();

            var usersById = await GetUsersByIdAsync(allTargetUserIds, cancellationToken);
            var conflicts = await _conflictService.FindExternalAppointmentConflictsAsync(
                currentEntry,
                cancellationToken);

            var changeLines = BuildChangeLines(
                previousEntry,
                currentEntry,
                addedParticipantIds,
                removedParticipantIds,
                usersById);

            if (changeLines.Count == 0 && addedParticipantIds.Count == 0 && removedParticipantIds.Count == 0 && conflicts.Count == 0)
            {
                return;
            }

            var changeId = Guid.NewGuid().ToString("N");

            var retainedUsers = GetUsers(retainedParticipantIds, usersById);
            if (retainedUsers.Count > 0 && (changeLines.Count > 0 || conflicts.Count > 0))
            {
                await SendDirectNotificationsAsync(
                    currentEntry,
                    retainedUsers,
                    ChatworkDeliveryTypes.WorkScheduleUpdated,
                    user => ChatworkDeliveryKeys.WorkScheduleUpdated(currentEntry.Id, user.Id, changeId),
                    BuildUpdatedMessage(currentEntry, changeLines, conflicts),
                    cancellationToken);
            }

            var addedUsers = GetUsers(addedParticipantIds, usersById);
            if (addedUsers.Count > 0)
            {
                await SendDirectNotificationsAsync(
                    currentEntry,
                    addedUsers,
                    ChatworkDeliveryTypes.WorkScheduleUpdated,
                    user => ChatworkDeliveryKeys.WorkScheduleUpdated(currentEntry.Id, user.Id, changeId),
                    BuildAddedParticipantMessage(currentEntry, conflicts),
                    cancellationToken);
            }

            var removedUsers = GetUsers(removedParticipantIds, usersById);
            if (removedUsers.Count > 0)
            {
                await SendDirectNotificationsAsync(
                    previousEntry,
                    removedUsers,
                    ChatworkDeliveryTypes.WorkScheduleUpdated,
                    user => ChatworkDeliveryKeys.WorkScheduleUpdated(previousEntry.Id, user.Id, changeId),
                    BuildRemovedParticipantMessage(previousEntry),
                    cancellationToken);
            }
        }

        public async Task SendDeletedAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            var targetUserIds = GetParticipantIds(entry);
            var usersById = await GetUsersByIdAsync(targetUserIds, cancellationToken);
            var targetUsers = GetUsers(targetUserIds, usersById);

            if (targetUsers.Count == 0)
            {
                return;
            }

            await SendDirectNotificationsAsync(
                entry,
                targetUsers,
                ChatworkDeliveryTypes.WorkScheduleDeleted,
                user => ChatworkDeliveryKeys.WorkScheduleDeleted(entry.Id, user.Id),
                BuildDeletedMessage(entry),
                cancellationToken);
        }

        public async Task SendReminderAsync(
            WorkScheduleEntryModel entry,
            CancellationToken cancellationToken = default)
        {
            if (entry.Type != WorkScheduleEntryType.ExternalAppointment ||
                !entry.StartTime.HasValue ||
                !entry.EndTime.HasValue)
            {
                return;
            }

            var targetUserIds = GetParticipantIds(entry);
            var usersById = await GetUsersByIdAsync(targetUserIds, cancellationToken);
            var targetUsers = GetUsers(targetUserIds, usersById);

            if (targetUsers.Count == 0)
            {
                return;
            }

            await SendDirectNotificationsAsync(
                entry,
                targetUsers,
                ChatworkDeliveryTypes.WorkScheduleReminder10Minutes,
                user => ChatworkDeliveryKeys.WorkScheduleReminder10Minutes(
                    entry.Id,
                    user.Id,
                    entry.StartTime.Value),
                BuildReminderMessage(entry),
                cancellationToken);
        }

        private async Task<Dictionary<int, UserModel>> GetUsersByIdAsync(
            IEnumerable<int> userIds,
            CancellationToken cancellationToken)
        {
            var distinctIds = userIds
                .Where(id => id > 0)
                .Distinct()
                .ToList();

            if (distinctIds.Count == 0)
            {
                return new Dictionary<int, UserModel>();
            }

            return await _context.Users
                .Where(user => distinctIds.Contains(user.Id))
                .ToDictionaryAsync(user => user.Id, cancellationToken);
        }

        private async Task SendDirectNotificationsAsync(
            WorkScheduleEntryModel entry,
            IReadOnlyCollection<UserModel> targetUsers,
            string deliveryType,
            Func<UserModel, string> buildDeliveryKey,
            string message,
            CancellationToken cancellationToken)
        {
            if (string.IsNullOrWhiteSpace(message) || targetUsers.Count == 0)
            {
                return;
            }

            var uniqueTargetUsers = targetUsers
                .Where(user => user.Id > 0)
                .GroupBy(user => user.Id)
                .Select(group => group.First())
                .ToList();

            var deliveryKeys = uniqueTargetUsers
                .Select(buildDeliveryKey)
                .ToList();

            var existingDeliveryKeyList = await _context.ChatworkDeliveryLogs
                .AsNoTracking()
                .Where(log => log.DeliveryKey != null && deliveryKeys.Contains(log.DeliveryKey))
                .Select(log => log.DeliveryKey!)
                .ToListAsync(cancellationToken);

            var existingDeliveryKeys = existingDeliveryKeyList.ToHashSet(StringComparer.Ordinal);

            foreach (var targetUser in uniqueTargetUsers)
            {
                var deliveryKey = buildDeliveryKey(targetUser);

                if (existingDeliveryKeys.Contains(deliveryKey))
                {
                    continue;
                }

                var attemptedAt = DateTime.Now;
                var roomId = targetUser.ChatworkDirectRoomId?.Trim();

                if (string.IsNullOrWhiteSpace(roomId))
                {
                    _context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = 0,
                        WorkScheduleEntryId = entry.Id,
                        DeliveryType = deliveryType,
                        DeliveryKey = deliveryKey,
                        TargetUserId = targetUser.Id,
                        ScheduledStartTime = GetScheduledStartTime(entry),
                        RoomId = null,
                        Status = ChatworkDeliveryStatuses.Skipped,
                        ErrorMessage = "ChatworkDirectRoomId is not configured.",
                        AttemptedAt = attemptedAt,
                        SentAt = null,
                        Message = message,
                        CreatedAt = attemptedAt
                    });

                    await _context.SaveChangesAsync(cancellationToken);
                    existingDeliveryKeys.Add(deliveryKey);
                    continue;
                }

                try
                {
                    await _chatworkClient.SendMessageAsync(roomId, message, cancellationToken);

                    var sentAt = DateTime.Now;

                    _context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = 0,
                        WorkScheduleEntryId = entry.Id,
                        DeliveryType = deliveryType,
                        DeliveryKey = deliveryKey,
                        TargetUserId = targetUser.Id,
                        ScheduledStartTime = GetScheduledStartTime(entry),
                        RoomId = roomId,
                        Status = ChatworkDeliveryStatuses.Succeeded,
                        ErrorMessage = null,
                        AttemptedAt = attemptedAt,
                        SentAt = sentAt,
                        Message = message,
                        CreatedAt = attemptedAt
                    });

                    await _context.SaveChangesAsync(cancellationToken);
                    existingDeliveryKeys.Add(deliveryKey);
                }
                catch (Exception ex)
                {
                    _logger.LogError(
                        ex,
                        "Failed to send direct Chatwork notification. WorkScheduleEntryId: {WorkScheduleEntryId}, DeliveryType: {DeliveryType}, UserId: {UserId}.",
                        entry.Id,
                        deliveryType,
                        targetUser.Id);

                    _context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = 0,
                        WorkScheduleEntryId = entry.Id,
                        DeliveryType = deliveryType,
                        DeliveryKey = deliveryKey,
                        TargetUserId = targetUser.Id,
                        ScheduledStartTime = GetScheduledStartTime(entry),
                        RoomId = roomId,
                        Status = ChatworkDeliveryStatuses.Failed,
                        ErrorMessage = ex.Message,
                        AttemptedAt = attemptedAt,
                        SentAt = null,
                        Message = message,
                        CreatedAt = attemptedAt
                    });

                    await _context.SaveChangesAsync(cancellationToken);
                    existingDeliveryKeys.Add(deliveryKey);
                }
            }
        }

        private static string BuildCreatedMessage(
            WorkScheduleEntryModel entry,
            IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
        {
            return BuildInfoMessage(
                $"{GetTypeLabel(entry.Type)}が登録されました",
                BuildSummaryLines(entry)
                    .Concat(BuildConflictLines(conflicts)));
        }

        private static string BuildUpdatedMessage(
            WorkScheduleEntryModel entry,
            IReadOnlyCollection<string> changeLines,
            IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
        {
            var lines = BuildSummaryLines(entry).ToList();

            if (changeLines.Count > 0)
            {
                lines.Add("[hr]");
                lines.Add("変更点:");
                lines.AddRange(changeLines.Select(change => $"- {change}"));
            }

            lines.AddRange(BuildConflictLines(conflicts));

            return BuildInfoMessage(
                $"{GetTypeLabel(entry.Type)}が変更されました",
                lines);
        }

        private static string BuildAddedParticipantMessage(
            WorkScheduleEntryModel entry,
            IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
        {
            return BuildInfoMessage(
                $"{GetTypeLabel(entry.Type)}の対象者に追加されました",
                BuildSummaryLines(entry)
                    .Concat(BuildConflictLines(conflicts)));
        }

        private static string BuildRemovedParticipantMessage(WorkScheduleEntryModel entry)
        {
            return BuildInfoMessage(
                $"{GetTypeLabel(entry.Type)}の対象者から削除されました",
                new[]
                {
                    $"内容: {entry.Title}",
                    $"日付: {entry.Date:yyyy/MM/dd}"
                });
        }

        private static string BuildDeletedMessage(WorkScheduleEntryModel entry)
        {
            return BuildInfoMessage(
                $"{GetTypeLabel(entry.Type)}が削除されました",
                BuildSummaryLines(entry));
        }

        private static string BuildReminderMessage(WorkScheduleEntryModel entry)
        {
            return BuildInfoMessage(
                "社外予定リマインド",
                new[]
                {
                    "10分後に開始します。",
                    $"内容: {entry.Title}",
                    $"日付: {entry.Date:yyyy/MM/dd}",
                    $"時間: {GetTimeRangeText(entry)}",
                    $"対象者: {GetParticipantText(entry)}"
                });
        }

        private static List<string> BuildSummaryLines(WorkScheduleEntryModel entry)
        {
            var lines = new List<string>
            {
                $"内容: {entry.Title}",
                $"日付: {entry.Date:yyyy/MM/dd}"
            };

            if (entry.Type == WorkScheduleEntryType.ExternalAppointment)
            {
                lines.Add($"時間: {GetTimeRangeText(entry)}");
            }

            if (entry.Type == WorkScheduleEntryType.Leave)
            {
                lines.Add($"休暇区分: {GetLeavePeriodLabel(entry.LeavePeriod)}");
            }

            lines.Add($"対象者: {GetParticipantText(entry)}");

            return lines;
        }

        private static List<string> BuildChangeLines(
            WorkScheduleEntryModel previousEntry,
            WorkScheduleEntryModel currentEntry,
            IReadOnlyCollection<int> addedParticipantIds,
            IReadOnlyCollection<int> removedParticipantIds,
            IReadOnlyDictionary<int, UserModel> usersById)
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

            var addedNames = FormatUserNames(addedParticipantIds, usersById);
            if (!string.IsNullOrWhiteSpace(addedNames))
            {
                changes.Add($"対象者に {addedNames} が追加されました");
            }

            var removedNames = FormatUserNames(removedParticipantIds, usersById);
            if (!string.IsNullOrWhiteSpace(removedNames))
            {
                changes.Add($"対象者から {removedNames} が削除されました");
            }

            return changes;
        }

        private static List<string> BuildConflictLines(
            IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts)
        {
            if (conflicts.Count == 0)
            {
                return new List<string>();
            }

            var lines = new List<string>
            {
                "[hr]",
                "注意: 参加者の予定重複があります"
            };

            var conflictLines = conflicts
                .GroupBy(conflict => new
                {
                    conflict.ParticipantUserId,
                    conflict.SourceType,
                    conflict.SourceId
                })
                .Select(group => group.First())
                .Take(5)
                .Select(conflict =>
                    $"- {conflict.ParticipantName}: {conflict.SourceType}「{conflict.SourceTitle}」 {conflict.Date:yyyy/MM/dd} {GetTimeRangeText(conflict.StartTime, conflict.EndTime)}")
                .ToList();

            lines.AddRange(conflictLines);

            if (conflicts.Count > 5)
            {
                lines.Add($"- ほか {conflicts.Count - 5}件");
            }

            return lines;
        }

        private static string BuildInfoMessage(
            string title,
            IEnumerable<string> lines)
        {
            var builder = new StringBuilder();
            builder.Append("[info][title]");
            builder.Append(title);
            builder.Append("[/title]");

            foreach (var line in lines.Where(line => !string.IsNullOrWhiteSpace(line)))
            {
                builder.Append('\n');
                builder.Append(line);
            }

            builder.Append("\n[/info]");
            return builder.ToString();
        }

        private static List<int> GetParticipantIds(WorkScheduleEntryModel entry)
        {
            return (entry.ParticipantIds ?? new List<int>())
                .Where(id => id > 0)
                .Distinct()
                .ToList();
        }

        private static List<UserModel> GetUsers(
            IEnumerable<int> userIds,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return userIds
                .Where(id => id > 0)
                .Distinct()
                .Select(id => usersById.TryGetValue(id, out var user) ? user : null)
                .Where(user => user is not null)
                .Cast<UserModel>()
                .ToList();
        }

        private static string FormatUserNames(
            IEnumerable<int> userIds,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            var names = userIds
                .Distinct()
                .Select(id => usersById.TryGetValue(id, out var user) ? user.Name : $"ユーザー{id}")
                .Where(name => !string.IsNullOrWhiteSpace(name))
                .ToList();

            return string.Join("、", names);
        }

        private static string GetParticipantText(WorkScheduleEntryModel entry)
        {
            return string.IsNullOrWhiteSpace(entry.Participants)
                ? "未設定"
                : entry.Participants;
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

        private static DateTime GetScheduledStartTime(WorkScheduleEntryModel entry)
        {
            return entry.StartTime ?? entry.Date.Date;
        }
    }
}

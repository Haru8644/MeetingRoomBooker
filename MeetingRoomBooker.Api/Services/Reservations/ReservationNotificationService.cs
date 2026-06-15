using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Reservations;

public interface IReservationNotificationService
{
    Task NotifyParticipantsForCreatedReservationAsync(ReservationModel reservation);

    Task NotifyParticipantsForUpdatedReservationAsync(
        ReservationModel previousReservation,
        ReservationModel reservation,
        IReadOnlyCollection<int> previousParticipantIds,
        IReadOnlyCollection<int> currentParticipantIds);

    Task NotifyOrganizerForParticipationChangedAsync(
        ReservationModel reservation,
        int actorUserId,
        bool isJoin,
        CancellationToken cancellationToken);
}

public sealed class ReservationNotificationService : IReservationNotificationService
{
    private const string InfoNotificationType = "Info";
    private const string WarningNotificationType = "Warning";

    private readonly AppDbContext _context;
    private readonly IReservationSeriesQueryService _reservationSeriesQueryService;
    private readonly IWorkScheduleParticipantConflictService _workScheduleParticipantConflictService;

    public ReservationNotificationService(
        AppDbContext context,
        IReservationSeriesQueryService reservationSeriesQueryService,
        IWorkScheduleParticipantConflictService workScheduleParticipantConflictService)
    {
        _context = context;
        _reservationSeriesQueryService = reservationSeriesQueryService;
        _workScheduleParticipantConflictService = workScheduleParticipantConflictService;
    }

    public async Task NotifyParticipantsForCreatedReservationAsync(ReservationModel reservation)
    {
        var targetUsers = ReservationRules.GetNotifiableParticipantIds(reservation);
        if (targetUsers.Count == 0)
        {
            return;
        }

        var participantConflicts = await _workScheduleParticipantConflictService
            .FindReservationExternalAppointmentConflictsAsync(reservation, CancellationToken.None);

        var notificationType = participantConflicts.Count > 0
            ? WarningNotificationType
            : InfoNotificationType;

        if (ReservationRules.IsRecurring(reservation))
        {
            await UpsertRecurringReservationNotificationsAsync(
                reservation,
                targetUsers,
                notificationType,
                participantConflicts);
            return;
        }

        foreach (var userId in targetUsers)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                BuildAddedParticipantMessage(reservation, participantConflicts),
                reservation.Date,
                reservation.Id);
        }
    }

    public async Task NotifyParticipantsForUpdatedReservationAsync(
        ReservationModel previousReservation,
        ReservationModel reservation,
        IReadOnlyCollection<int> previousParticipantIds,
        IReadOnlyCollection<int> currentParticipantIds)
    {
        var retainedParticipantIds = currentParticipantIds.Intersect(previousParticipantIds).ToList();
        var addedParticipantIds = currentParticipantIds.Except(previousParticipantIds).ToList();
        var removedParticipantIds = previousParticipantIds.Except(currentParticipantIds).ToList();

        var participantConflicts = await _workScheduleParticipantConflictService
            .FindReservationExternalAppointmentConflictsAsync(reservation, CancellationToken.None);

        var notificationType = participantConflicts.Count > 0
            ? WarningNotificationType
            : InfoNotificationType;

        var detailedMessage = await BuildReservationUpdatedMessageAsync(
            previousReservation,
            reservation,
            addedParticipantIds,
            removedParticipantIds,
            participantConflicts);

        foreach (var userId in addedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                BuildAddedParticipantMessage(reservation, participantConflicts),
                reservation.Date,
                reservation.Id);
        }

        foreach (var userId in removedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                InfoNotificationType,
                BuildRemovedParticipantMessage(reservation),
                reservation.Date,
                reservation.Id);
        }

        if (string.IsNullOrWhiteSpace(detailedMessage))
        {
            return;
        }

        foreach (var userId in retainedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                detailedMessage,
                reservation.Date,
                reservation.Id);
        }
    }

    public async Task NotifyOrganizerForParticipationChangedAsync(
        ReservationModel reservation,
        int actorUserId,
        bool isJoin,
        CancellationToken cancellationToken)
    {
        if (reservation.UserId == actorUserId)
        {
            return;
        }

        var actorName = await _context.Users
            .Where(x => x.Id == actorUserId)
            .Select(x => x.Name)
            .FirstOrDefaultAsync(cancellationToken)
            ?? $"ユーザー{actorUserId}";

        var message = isJoin
            ? $"{actorName}さんが予約「{GetReservationLabel(reservation)}」への参加を追加しました。利用日: {reservation.Date:yyyy/MM/dd} / 会議室: {reservation.Room} / 時間: {GetTimeRangeText(reservation)}"
            : $"{actorName}さんが予約「{GetReservationLabel(reservation)}」への参加を取り消しました。利用日: {reservation.Date:yyyy/MM/dd} / 会議室: {reservation.Room} / 時間: {GetTimeRangeText(reservation)}";

        await UpsertNotificationAsync(
            reservation.UserId,
            InfoNotificationType,
            message,
            reservation.Date,
            reservation.Id,
            $"{actorName}さんが予約「{GetReservationLabel(reservation)}」");
    }

    private async Task UpsertRecurringReservationNotificationsAsync(
        ReservationModel reservation,
        IReadOnlyCollection<int> targetUsers,
        string notificationType,
        IReadOnlyCollection<WorkScheduleParticipantConflict> participantConflicts)
    {
        var seriesReservations = await _reservationSeriesQueryService.GetSeriesReservationsAsync(
            reservation,
            ReservationSeriesScopes.All,
            CancellationToken.None);

        var firstReservation = seriesReservations.FirstOrDefault();
        if (firstReservation == null)
        {
            return;
        }

        var count = seriesReservations.Count;
        var message = $"{reservation.Name}さんが繰り返し予約「{GetReservationLabel(reservation)}」にあなたを追加しました。({count}件){BuildConflictText(participantConflicts)}";
        var messagePrefix = GetRecurringNotificationPrefix(reservation);

        foreach (var userId in targetUsers)
        {
            await UpsertNotificationAsync(
                userId,
                notificationType,
                message,
                firstReservation.Date,
                firstReservation.Id,
                messagePrefix);
        }
    }

    private async Task<string?> BuildReservationUpdatedMessageAsync(
        ReservationModel previousReservation,
        ReservationModel reservation,
        IReadOnlyCollection<int> addedParticipantIds,
        IReadOnlyCollection<int> removedParticipantIds,
        IReadOnlyCollection<WorkScheduleParticipantConflict> participantConflicts)
    {
        var changes = new List<string>();

        if (previousReservation.Date.Date != reservation.Date.Date)
        {
            changes.Add($"利用日が {previousReservation.Date:yyyy/MM/dd} から {reservation.Date:yyyy/MM/dd} に変更されました");
        }

        if (!string.Equals(previousReservation.Room, reservation.Room, StringComparison.Ordinal))
        {
            changes.Add($"会議室が {previousReservation.Room} から {reservation.Room} に変更されました");
        }

        if (!string.Equals(previousReservation.Type, reservation.Type, StringComparison.Ordinal))
        {
            changes.Add($"区分が {previousReservation.Type} から {reservation.Type} に変更されました");
        }

        if (previousReservation.StartTime.TimeOfDay != reservation.StartTime.TimeOfDay
            || previousReservation.EndTime.TimeOfDay != reservation.EndTime.TimeOfDay)
        {
            changes.Add($"時間が {GetTimeRangeText(previousReservation)} から {GetTimeRangeText(reservation)} に変更されました");
        }

        var previousLabel = GetReservationLabel(previousReservation);
        var currentLabel = GetReservationLabel(reservation);
        if (!string.Equals(previousLabel, currentLabel, StringComparison.Ordinal))
        {
            changes.Add($"予約名が「{previousLabel}」から「{currentLabel}」に変更されました");
        }

        var participantNames = await GetUserNamesByIdAsync(addedParticipantIds.Concat(removedParticipantIds));
        var addedNames = FormatUserNames(addedParticipantIds, participantNames);
        if (!string.IsNullOrWhiteSpace(addedNames))
        {
            changes.Add($"参加メンバーに {addedNames} が追加されました");
        }

        var removedNames = FormatUserNames(removedParticipantIds, participantNames);
        if (!string.IsNullOrWhiteSpace(removedNames))
        {
            changes.Add($"参加メンバーから {removedNames} が削除されました");
        }

        if (changes.Count == 0 && participantConflicts.Count == 0)
        {
            return null;
        }

        var notificationLabel = !string.IsNullOrWhiteSpace(previousLabel) ? previousLabel : currentLabel;
        if (changes.Count == 0)
        {
            return $"予約「{notificationLabel}」に参加者の予定重複があります。{BuildConflictText(participantConflicts)}";
        }

        return $"予約「{notificationLabel}」が更新されました。{string.Join(" ", changes.Select(change => $"{change}。"))}{BuildConflictText(participantConflicts)}";
    }

    private async Task<Dictionary<int, string>> GetUserNamesByIdAsync(IEnumerable<int> userIds)
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
            .Where(user => distinctIds.Contains(user.Id))
            .ToDictionaryAsync(user => user.Id, user => user.Name);
    }

    private async Task UpsertNotificationAsync(
        int userId,
        string type,
        string message,
        DateTime targetDate,
        int targetReservationId,
        string? messagePrefix = null)
    {
        var candidates = await _context.Notifications
            .Where(n =>
                n.UserId == userId &&
                n.Type == type &&
                n.TargetReservationId == targetReservationId)
            .OrderByDescending(n => n.CreatedAt)
            .ToListAsync();

        var matchingNotifications = string.IsNullOrWhiteSpace(messagePrefix)
            ? candidates.Where(n => n.Message == message).ToList()
            : candidates.Where(n => n.Message.StartsWith(messagePrefix, StringComparison.Ordinal)).ToList();

        var existing = matchingNotifications.FirstOrDefault();

        if (existing == null)
        {
            _context.Notifications.Add(new NotificationModel
            {
                UserId = userId,
                Type = type,
                Message = message,
                TargetDate = targetDate,
                TargetReservationId = targetReservationId,
                TargetWorkScheduleEntryId = null,
                CreatedAt = DateTime.Now,
                IsRead = false
            });

            return;
        }

        existing.Message = message;
        existing.TargetDate = targetDate;
        existing.CreatedAt = DateTime.Now;
        existing.IsRead = false;

        if (matchingNotifications.Count > 1)
        {
            _context.Notifications.RemoveRange(matchingNotifications.Skip(1));
        }
    }

    private static string GetReservationLabel(ReservationModel reservation)
    {
        return string.IsNullOrWhiteSpace(reservation.Purpose)
            ? reservation.Name
            : reservation.Purpose;
    }

    private static string GetTimeRangeText(ReservationModel reservation)
    {
        return $"{reservation.StartTime:HH:mm}〜{reservation.EndTime:HH:mm}";
    }

    private static string BuildAddedParticipantMessage(
        ReservationModel reservation,
        IReadOnlyCollection<WorkScheduleParticipantConflict> participantConflicts)
    {
        return $"{reservation.Name}さんが予約「{GetReservationLabel(reservation)}」の参加メンバーにあなたを追加しました。利用日: {reservation.Date:yyyy/MM/dd} / 会議室: {reservation.Room} / 時間: {GetTimeRangeText(reservation)}{BuildConflictText(participantConflicts)}";
    }

    private static string BuildRemovedParticipantMessage(ReservationModel reservation)
    {
        return $"{reservation.Name}さんが予約「{GetReservationLabel(reservation)}」の参加メンバーからあなたを削除しました。";
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

    private static string GetTimeRangeText(DateTime? startTime, DateTime? endTime)
    {
        return startTime.HasValue && endTime.HasValue
            ? $"{startTime.Value:HH:mm}〜{endTime.Value:HH:mm}"
            : "終日";
    }

    private static string GetRecurringNotificationPrefix(ReservationModel reservation)
    {
        return $"{reservation.Name}さんが繰り返し予約「{GetReservationLabel(reservation)}」にあなたを追加しました。(";
    }

    private static string FormatUserNames(IEnumerable<int> userIds, IReadOnlyDictionary<int, string> userNamesById)
    {
        var names = userIds
            .Distinct()
            .Select(id => userNamesById.TryGetValue(id, out var name) ? name : $"ユーザー{id}")
            .Where(name => !string.IsNullOrWhiteSpace(name))
            .ToList();

        return string.Join("、", names);
    }
}

using MeetingRoomBooker.Api.Data;
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

    private readonly AppDbContext _context;
    private readonly IReservationSeriesQueryService _reservationSeriesQueryService;

    public ReservationNotificationService(
        AppDbContext context,
        IReservationSeriesQueryService reservationSeriesQueryService)
    {
        _context = context;
        _reservationSeriesQueryService = reservationSeriesQueryService;
    }

    public async Task NotifyParticipantsForCreatedReservationAsync(ReservationModel reservation)
    {
        var targetUsers = ReservationRules.GetNotifiableParticipantIds(reservation);
        if (targetUsers.Count == 0)
        {
            return;
        }

        if (ReservationRules.IsRecurring(reservation))
        {
            await UpsertRecurringReservationNotificationsAsync(reservation, targetUsers);
            return;
        }

        foreach (var userId in targetUsers)
        {
            await UpsertNotificationAsync(
                userId,
                InfoNotificationType,
                BuildAddedParticipantMessage(reservation),
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

        var detailedMessage = await BuildReservationUpdatedMessageAsync(
            previousReservation,
            reservation,
            addedParticipantIds,
            removedParticipantIds);

        foreach (var userId in addedParticipantIds)
        {
            await UpsertNotificationAsync(
                userId,
                InfoNotificationType,
                BuildAddedParticipantMessage(reservation),
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
                InfoNotificationType,
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
        IReadOnlyCollection<int> targetUsers)
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
        var message = $"{reservation.Name}さんが繰り返し予約「{GetReservationLabel(reservation)}」にあなたを追加しました。({count}件)";
        var messagePrefix = GetRecurringNotificationPrefix(reservation);

        foreach (var userId in targetUsers)
        {
            await UpsertNotificationAsync(
                userId,
                InfoNotificationType,
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
        IReadOnlyCollection<int> removedParticipantIds)
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

        if (changes.Count == 0)
        {
            return null;
        }

        var notificationLabel = !string.IsNullOrWhiteSpace(previousLabel) ? previousLabel : currentLabel;
        return $"予約「{notificationLabel}」が更新されました。{string.Join(" ", changes.Select(change => $"{change}。"))}";
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

    private static string BuildAddedParticipantMessage(ReservationModel reservation)
    {
        return $"{reservation.Name}さんが予約「{GetReservationLabel(reservation)}」の参加メンバーにあなたを追加しました。利用日: {reservation.Date:yyyy/MM/dd} / 会議室: {reservation.Room} / 時間: {GetTimeRangeText(reservation)}";
    }

    private static string BuildRemovedParticipantMessage(ReservationModel reservation)
    {
        return $"{reservation.Name}さんが予約「{GetReservationLabel(reservation)}」の参加メンバーからあなたを削除しました。";
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

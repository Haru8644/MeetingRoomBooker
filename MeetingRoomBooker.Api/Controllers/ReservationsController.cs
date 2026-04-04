using System.Security.Claims;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Controllers
{
    [Route("api/[controller]")]
    [ApiController]
    public class ReservationsController : ControllerBase
    {
        private const string InfoNotificationType = "Info";
        private const string WarningNotificationType = "Warning";

        private readonly AppDbContext _context;
        private readonly IReservationChatworkNotificationService _reservationChatworkNotificationService;

        public ReservationsController(
            AppDbContext context,
            IReservationChatworkNotificationService reservationChatworkNotificationService)
        {
            _context = context;
            _reservationChatworkNotificationService = reservationChatworkNotificationService;
        }

        [HttpGet]
        public async Task<ActionResult<IEnumerable<ReservationModel>>> GetReservations(CancellationToken cancellationToken)
        {
            return await _context.Reservations.ToListAsync(cancellationToken);
        }

        [HttpPost]
        public async Task<ActionResult<ReservationModel>> PostReservation(
            ReservationModel reservation,
            CancellationToken cancellationToken)
        {
            NormalizeReservation(reservation);

            _context.Reservations.Add(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await NotifyParticipantsForCreatedReservationAsync(reservation);
            await NotifyConflictOwnersAsync(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationCreatedAsync(
                reservation,
                cancellationToken);

            return CreatedAtAction(nameof(GetReservations), new { id = reservation.Id }, reservation);
        }

        [HttpDelete("{id}")]
        public async Task<IActionResult> DeleteReservation(int id, CancellationToken cancellationToken)
        {
            var reservation = await _context.Reservations
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

            if (reservation == null)
            {
                return NotFound();
            }

            _context.Reservations.Remove(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationCanceledAsync(
                reservation,
                cancellationToken);

            return NoContent();
        }

        [HttpPut("{id}")]
        public async Task<IActionResult> PutReservation(
            int id,
            ReservationModel reservation,
            [FromQuery] bool notifyParticipants = true,
            CancellationToken cancellationToken = default)
        {
            if (id != reservation.Id)
            {
                return BadRequest();
            }

            var existingReservation = await _context.Reservations
                .AsNoTracking()
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

            if (existingReservation == null)
            {
                return NotFound();
            }

            NormalizeReservation(reservation);
            reservation.UserId = existingReservation.UserId;

            var previousParticipantIds = GetNotifiableParticipantIds(existingReservation);
            var currentParticipantIds = GetNotifiableParticipantIds(reservation);

            _context.Entry(reservation).State = EntityState.Modified;

            try
            {
                await _context.SaveChangesAsync(cancellationToken);
            }
            catch (DbUpdateConcurrencyException)
            {
                if (!_context.Reservations.Any(e => e.Id == id))
                {
                    return NotFound();
                }

                throw;
            }

            if (notifyParticipants)
            {
                await NotifyParticipantsForUpdatedReservationAsync(
                    existingReservation,
                    reservation,
                    previousParticipantIds,
                    currentParticipantIds);
            }

            await NotifyConflictOwnersAsync(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationUpdatedAsync(
                existingReservation,
                reservation,
                cancellationToken);

            return NoContent();
        }

        [Authorize]
        [HttpPost("{id}/join")]
        public async Task<IActionResult> JoinReservation(int id, CancellationToken cancellationToken)
        {
            var reservation = await _context.Reservations.FirstOrDefaultAsync(x => x.Id == id, cancellationToken);
            if (reservation == null)
            {
                return NotFound();
            }

            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return Unauthorized();
            }

            reservation.ParticipantIds ??= new List<int>();
            if (reservation.ParticipantIds.Contains(currentUserId))
            {
                return NoContent();
            }

            reservation.ParticipantIds.Add(currentUserId);
            NormalizeReservation(reservation);

            await _context.SaveChangesAsync(cancellationToken);
            await NotifyOrganizerForParticipationChangedAsync(reservation, currentUserId, true, cancellationToken);
            await _context.SaveChangesAsync(cancellationToken);

            return NoContent();
        }

        [Authorize]
        [HttpPost("{id}/leave")]
        public async Task<IActionResult> LeaveReservation(int id, CancellationToken cancellationToken)
        {
            var reservation = await _context.Reservations.FirstOrDefaultAsync(x => x.Id == id, cancellationToken);
            if (reservation == null)
            {
                return NotFound();
            }

            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return Unauthorized();
            }

            if (reservation.UserId == currentUserId)
            {
                return BadRequest("予約者本人は参加メンバーから外せません。");
            }

            if (reservation.ParticipantIds == null || !reservation.ParticipantIds.Remove(currentUserId))
            {
                return NoContent();
            }

            NormalizeReservation(reservation);

            await _context.SaveChangesAsync(cancellationToken);
            await NotifyOrganizerForParticipationChangedAsync(reservation, currentUserId, false, cancellationToken);
            await _context.SaveChangesAsync(cancellationToken);

            return NoContent();
        }

        [Authorize]
        [HttpPost("{id}/series-delete")]
        public async Task<IActionResult> DeleteReservationSeries(
            int id,
            ReservationSeriesDeleteRequest request,
            CancellationToken cancellationToken)
        {
            var baseReservation = await _context.Reservations
                .AsNoTracking()
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

            if (baseReservation == null)
            {
                return NotFound();
            }

            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return Unauthorized();
            }

            if (baseReservation.UserId != currentUserId)
            {
                return Forbid();
            }

            var scope = NormalizeSeriesScope(request.Scope);
            if (scope == ReservationSeriesScopes.Single || !IsRecurringReservation(baseReservation))
            {
                var single = await _context.Reservations.FirstOrDefaultAsync(x => x.Id == id, cancellationToken);
                if (single == null)
                {
                    return NotFound();
                }

                _context.Reservations.Remove(single);
                await _context.SaveChangesAsync(cancellationToken);
                return NoContent();
            }

            var targets = await GetRecurringSeriesReservationsAsync(baseReservation, scope, cancellationToken);
            if (targets.Count == 0)
            {
                return NoContent();
            }

            _context.Reservations.RemoveRange(targets);
            await _context.SaveChangesAsync(cancellationToken);

            return NoContent();
        }

        [Authorize]
        [HttpPost("{id}/series-update")]
        public async Task<IActionResult> UpdateReservationSeries(
            int id,
            ReservationSeriesUpdateRequest request,
            CancellationToken cancellationToken)
        {
            var baseReservation = await _context.Reservations
                .AsNoTracking()
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

            if (baseReservation == null)
            {
                return NotFound();
            }

            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return Unauthorized();
            }

            if (baseReservation.UserId != currentUserId)
            {
                return Forbid();
            }

            var scope = NormalizeSeriesScope(request.Scope);
            if (scope == ReservationSeriesScopes.Single || !IsRecurringReservation(baseReservation))
            {
                return await PutReservation(id, request.UpdatedReservation, request.NotifyParticipants, cancellationToken);
            }

            var updatedTemplate = request.UpdatedReservation ?? new ReservationModel();
            NormalizeReservation(updatedTemplate);
            updatedTemplate.UserId = baseReservation.UserId;

            var targets = await GetRecurringSeriesReservationsAsync(baseReservation, scope, cancellationToken);
            if (targets.Count == 0)
            {
                return NoContent();
            }

            var dayOffset = updatedTemplate.Date.Date - baseReservation.Date.Date;
            var operations = new List<(ReservationModel Previous, ReservationModel Current, List<int> PreviousParticipantIds, List<int> CurrentParticipantIds)>();

            foreach (var target in targets)
            {
                var nextDate = target.Date.Date.Add(dayOffset);
                var updatedReservation = CreateRecurringUpdatedReservation(target, updatedTemplate, nextDate);
                var previousParticipantIds = GetNotifiableParticipantIds(target);
                var currentParticipantIds = GetNotifiableParticipantIds(updatedReservation);

                _context.Entry(updatedReservation).State = EntityState.Modified;
                operations.Add((target, updatedReservation, previousParticipantIds, currentParticipantIds));
            }

            await _context.SaveChangesAsync(cancellationToken);

            if (request.NotifyParticipants)
            {
                foreach (var operation in operations)
                {
                    await NotifyParticipantsForUpdatedReservationAsync(
                        operation.Previous,
                        operation.Current,
                        operation.PreviousParticipantIds,
                        operation.CurrentParticipantIds);
                }
            }

            foreach (var operation in operations)
            {
                await NotifyConflictOwnersAsync(operation.Current);
            }

            await _context.SaveChangesAsync(cancellationToken);

            return NoContent();
        }

        private async Task NotifyParticipantsForCreatedReservationAsync(ReservationModel reservation)
        {
            var targetUsers = GetNotifiableParticipantIds(reservation);
            if (targetUsers.Count == 0)
            {
                return;
            }

            if (IsRecurringReservation(reservation))
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

        private async Task NotifyParticipantsForUpdatedReservationAsync(
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

        private async Task NotifyConflictOwnersAsync(ReservationModel reservation)
        {
            var conflicts = await _context.Reservations
                .Where(x =>
                    x.Id != reservation.Id &&
                    x.Room == reservation.Room &&
                    x.Date.Date == reservation.Date.Date &&
                    x.StartTime < reservation.EndTime &&
                    x.EndTime > reservation.StartTime)
                .ToListAsync();

            foreach (var conflict in conflicts)
            {
                if (conflict.UserId == reservation.UserId)
                {
                    continue;
                }

                await UpsertNotificationAsync(
                    conflict.UserId,
                    WarningNotificationType,
                    $"【警告】{reservation.Date:MM/dd}の「{conflict.Purpose}」と時間が重複しました。",
                    reservation.Date,
                    reservation.Id);
            }
        }

        private async Task NotifyOrganizerForParticipationChangedAsync(
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

        private static bool IsRecurringReservation(ReservationModel reservation)
        {
            return !string.IsNullOrWhiteSpace(reservation.RepeatType)
                && reservation.RepeatType != "しない"
                && reservation.RepeatUntil.HasValue;
        }

        private async Task<List<ReservationModel>> GetRecurringSeriesReservationsAsync(
            ReservationModel reservation,
            string scope,
            CancellationToken cancellationToken)
        {
            var candidates = await _context.Reservations
                .AsNoTracking()
                .Where(x =>
                    x.UserId == reservation.UserId &&
                    x.Name == reservation.Name &&
                    x.Room == reservation.Room &&
                    x.Type == reservation.Type &&
                    x.Purpose == reservation.Purpose &&
                    x.RepeatType == reservation.RepeatType)
                .ToListAsync(cancellationToken);

            var matches = candidates
                .Where(x =>
                    x.RepeatUntil?.Date == reservation.RepeatUntil?.Date &&
                    x.StartTime.TimeOfDay == reservation.StartTime.TimeOfDay &&
                    x.EndTime.TimeOfDay == reservation.EndTime.TimeOfDay)
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .ToList();

            return scope == ReservationSeriesScopes.Following
                ? matches.Where(x => x.Date.Date >= reservation.Date.Date).ToList()
                : matches;
        }

        private async Task UpsertRecurringReservationNotificationsAsync(
            ReservationModel reservation,
            IReadOnlyCollection<int> targetUsers)
        {
            var seriesReservations = await GetRecurringSeriesReservationsAsync(
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

        private static List<int> GetNotifiableParticipantIds(ReservationModel reservation)
        {
            return (reservation.ParticipantIds ?? new List<int>())
                .Where(id => id > 0 && id != reservation.UserId)
                .Distinct()
                .ToList();
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

        private static void NormalizeReservation(ReservationModel reservation)
        {
            reservation.ParticipantIds = (reservation.ParticipantIds ?? new List<int>())
                .Where(id => id > 0)
                .Distinct()
                .ToList();

            if (reservation.UserId > 0 && !reservation.ParticipantIds.Contains(reservation.UserId))
            {
                reservation.ParticipantIds.Insert(0, reservation.UserId);
            }

            reservation.StartTime = reservation.Date.Date + reservation.StartTime.TimeOfDay;
            reservation.EndTime = reservation.Date.Date + reservation.EndTime.TimeOfDay;

            if (string.IsNullOrWhiteSpace(reservation.RepeatType))
            {
                reservation.RepeatType = "しない";
            }

            if (reservation.RepeatType == "しない")
            {
                reservation.RepeatUntil = null;
            }
        }

        private bool TryGetCurrentUserId(out int userId)
        {
            var userIdClaim = User.FindFirstValue(ClaimTypes.NameIdentifier);
            return int.TryParse(userIdClaim, out userId);
        }

        private static string NormalizeSeriesScope(string? scope)
        {
            return scope switch
            {
                ReservationSeriesScopes.All => ReservationSeriesScopes.All,
                ReservationSeriesScopes.Following => ReservationSeriesScopes.Following,
                _ => ReservationSeriesScopes.Single
            };
        }

        private static ReservationModel CreateRecurringUpdatedReservation(
            ReservationModel source,
            ReservationModel updatedTemplate,
            DateTime nextDate)
        {
            return new ReservationModel
            {
                Id = source.Id,
                UserId = source.UserId,
                Name = updatedTemplate.Name,
                Room = updatedTemplate.Room,
                NumberOfPeople = updatedTemplate.NumberOfPeople,
                Type = updatedTemplate.Type,
                Purpose = updatedTemplate.Purpose,
                Date = nextDate,
                StartTime = nextDate.Date + updatedTemplate.StartTime.TimeOfDay,
                EndTime = nextDate.Date + updatedTemplate.EndTime.TimeOfDay,
                ParticipantIds = (updatedTemplate.ParticipantIds ?? new List<int>()).Distinct().ToList(),
                Participants = updatedTemplate.Participants,
                RepeatType = source.RepeatType,
                RepeatUntil = source.RepeatUntil
            };
        }
    }
}

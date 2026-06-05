using System.Security.Claims;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Api.Services.Reservations;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Controllers
{
    [Route("api/[controller]")]
    [ApiController]
    [Authorize]
    public sealed class ReservationsController : ControllerBase
    {
        private const string InfoNotificationType = "Info";

        private readonly AppDbContext _context;
        private readonly IReservationChatworkNotificationService _reservationChatworkNotificationService;
        private readonly IReservationConflictService _reservationConflictService;
        private readonly IReservationSeriesQueryService _reservationSeriesQueryService;

        public ReservationsController(
            AppDbContext context,
            IReservationChatworkNotificationService reservationChatworkNotificationService,
            IReservationConflictService reservationConflictService,
            IReservationSeriesQueryService reservationSeriesQueryService)
        {
            _context = context;
            _reservationChatworkNotificationService = reservationChatworkNotificationService;
            _reservationConflictService = reservationConflictService;
            _reservationSeriesQueryService = reservationSeriesQueryService;
        }

        [HttpGet]
        public async Task<ActionResult<IEnumerable<ReservationModel>>> GetReservations(CancellationToken cancellationToken)
        {
            return await _context.Reservations
                .AsNoTracking()
                .ToListAsync(cancellationToken);
        }

        [HttpPost]
        public async Task<ActionResult<ReservationModel>> PostReservation(
            ReservationModel reservation,
            [FromQuery] bool allowOverlap,
            CancellationToken cancellationToken)
        {
            var currentUser = await GetCurrentUserAsync(cancellationToken);
            if (currentUser == null)
            {
                return Unauthorized();
            }

            reservation.UserId = currentUser.Id;
            reservation.Name = currentUser.Name;
            ReservationRules.Normalize(reservation);
            ReservationRules.SetSeriesIdForCreated(reservation);

            if (ReservationRules.IsRecurring(reservation) && ReservationRules.IsWeekend(reservation.Date))
            {
                return BadRequest("繰り返し予約では土日を登録できません。");
            }

            var validationError = ReservationRules.Validate(reservation);
            if (!string.IsNullOrWhiteSpace(validationError))
            {
                return BadRequest(validationError);
            }

            var overlappingReservations = await _reservationConflictService.FindConflictsAsync(
                reservation,
                cancellationToken: cancellationToken);

            if (overlappingReservations.Count > 0 && !allowOverlap)
            {
                return Conflict(_reservationConflictService.BuildConflictMessage(overlappingReservations[0]));
            }

            _context.Reservations.Add(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await NotifyParticipantsForCreatedReservationAsync(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationCreatedAsync(
                reservation,
                overlappingReservations,
                cancellationToken);

            return CreatedAtAction(nameof(GetReservations), new { id = reservation.Id }, reservation);
        }

        [HttpPost("series")]
        public async Task<ActionResult<IEnumerable<ReservationModel>>> PostReservationSeries(
            List<ReservationModel>? reservations,
            [FromQuery] bool allowOverlap,
            CancellationToken cancellationToken)
        {
            if (reservations is null || reservations.Count == 0)
            {
                return BadRequest("登録する予約がありません。");
            }

            var currentUser = await GetCurrentUserAsync(cancellationToken);
            if (currentUser == null)
            {
                return Unauthorized();
            }

            var seriesId = Guid.NewGuid().ToString("N");
            var normalizedReservations = new List<ReservationModel>();
            var overlappingReservations = new List<ReservationModel>();

            foreach (var reservation in reservations.OrderBy(x => x.Date).ThenBy(x => x.StartTime))
            {
                reservation.UserId = currentUser.Id;
                reservation.Name = currentUser.Name;
                ReservationRules.Normalize(reservation);
                ReservationRules.SetSeriesIdForCreated(reservation, seriesId);

                if (ReservationRules.IsRecurring(reservation) && ReservationRules.IsWeekend(reservation.Date))
                {
                    return BadRequest("繰り返し予約では土日を登録できません。");
                }

                var validationError = ReservationRules.Validate(reservation);
                if (!string.IsNullOrWhiteSpace(validationError))
                {
                    return BadRequest(validationError);
                }

                var conflictInRequest = normalizedReservations.FirstOrDefault(existing =>
                    ReservationOverlapChecker.IsConflicting(existing, reservation));
                if (conflictInRequest != null && !allowOverlap)
                {
                    return Conflict(_reservationConflictService.BuildConflictMessage(conflictInRequest));
                }

                var conflicts = await _reservationConflictService.FindConflictsAsync(
                    reservation,
                    cancellationToken: cancellationToken);

                if (conflicts.Count > 0 && !allowOverlap)
                {
                    return Conflict(_reservationConflictService.BuildConflictMessage(conflicts[0]));
                }

                overlappingReservations.AddRange(conflicts);
                normalizedReservations.Add(reservation);
            }

            _context.Reservations.AddRange(normalizedReservations);
            await _context.SaveChangesAsync(cancellationToken);

            var representativeReservation = ReservationRules.BuildSeriesRepresentative(normalizedReservations);

            await NotifyParticipantsForCreatedReservationAsync(representativeReservation);
            await _context.SaveChangesAsync(cancellationToken);

            var distinctOverlappingReservations = overlappingReservations
                .Where(x => x.Id > 0)
                .GroupBy(x => x.Id)
                .Select(group => group.First())
                .OrderBy(x => x.Date)
                .ThenBy(x => x.StartTime)
                .ToList();

            await _reservationChatworkNotificationService.SendReservationSeriesCreatedAsync(
                representativeReservation,
                normalizedReservations.Count,
                distinctOverlappingReservations,
                cancellationToken);

            return CreatedAtAction(nameof(GetReservations), routeValues: null, value: normalizedReservations);
        }

        [HttpDelete("{id:int}")]
        public async Task<IActionResult> DeleteReservation(int id, CancellationToken cancellationToken)
        {
            var reservation = await _context.Reservations
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

            if (reservation == null)
            {
                return NotFound();
            }

            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return Unauthorized();
            }

            if (!CanManageReservation(reservation, currentUserId))
            {
                return Forbid();
            }

            var canceledReservation = ReservationRules.Clone(reservation);

            _context.Reservations.Remove(reservation);
            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationCanceledAsync(
                canceledReservation,
                cancellationToken);

            return NoContent();
        }

        [HttpPut("{id:int}")]
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

            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return Unauthorized();
            }

            if (!CanManageReservation(existingReservation, currentUserId))
            {
                return Forbid();
            }

            ReservationRules.Normalize(reservation);
            reservation.UserId = existingReservation.UserId;
            reservation.Name = existingReservation.Name;
            reservation.SeriesId = existingReservation.SeriesId;

            var validationError = ReservationRules.Validate(reservation);
            if (!string.IsNullOrWhiteSpace(validationError))
            {
                return BadRequest(validationError);
            }

            var conflict = await _reservationConflictService.FindFirstConflictAsync(
                reservation,
                excludedReservationIds: new[] { reservation.Id },
                cancellationToken: cancellationToken);

            if (conflict != null)
            {
                return Conflict(_reservationConflictService.BuildConflictMessage(conflict));
            }

            var previousParticipantIds = ReservationRules.GetNotifiableParticipantIds(existingReservation);
            var currentParticipantIds = ReservationRules.GetNotifiableParticipantIds(reservation);

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

            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationUpdatedAsync(
                existingReservation,
                reservation,
                cancellationToken);

            return NoContent();
        }

        [HttpPost("{id:int}/join")]
        public async Task<IActionResult> JoinReservation(int id, CancellationToken cancellationToken)
        {
            var reservation = await _context.Reservations
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

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
            ReservationRules.Normalize(reservation);

            await _context.SaveChangesAsync(cancellationToken);
            await NotifyOrganizerForParticipationChangedAsync(reservation, currentUserId, true, cancellationToken);
            await _context.SaveChangesAsync(cancellationToken);

            return NoContent();
        }

        [HttpPost("{id:int}/leave")]
        public async Task<IActionResult> LeaveReservation(int id, CancellationToken cancellationToken)
        {
            var reservation = await _context.Reservations
                .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

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

            ReservationRules.Normalize(reservation);

            await _context.SaveChangesAsync(cancellationToken);
            await NotifyOrganizerForParticipationChangedAsync(reservation, currentUserId, false, cancellationToken);
            await _context.SaveChangesAsync(cancellationToken);

            return NoContent();
        }

        [HttpPost("{id:int}/series-delete")]
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

            if (!CanManageReservation(baseReservation, currentUserId))
            {
                return Forbid();
            }

            var scope = ReservationRules.NormalizeSeriesScope(request.Scope);
            if (scope == ReservationSeriesScopes.Single || !ReservationRules.IsRecurring(baseReservation))
            {
                var single = await _context.Reservations
                    .FirstOrDefaultAsync(x => x.Id == id, cancellationToken);

                if (single == null)
                {
                    return NotFound();
                }

                var canceledReservation = ReservationRules.Clone(single);

                _context.Reservations.Remove(single);
                await _context.SaveChangesAsync(cancellationToken);

                await _reservationChatworkNotificationService.SendReservationCanceledAsync(
                    canceledReservation,
                    cancellationToken);

                return NoContent();
            }

            var targets = await _reservationSeriesQueryService.GetSeriesReservationsAsync(
                baseReservation,
                scope,
                cancellationToken);

            if (targets.Count == 0)
            {
                return NoContent();
            }

            var representativeReservation = ReservationRules.BuildCanceledSeriesRepresentative(baseReservation, targets);

            _context.Reservations.RemoveRange(targets);
            await _context.SaveChangesAsync(cancellationToken);

            await _reservationChatworkNotificationService.SendReservationSeriesCanceledAsync(
                representativeReservation,
                targets.Count,
                cancellationToken);

            return NoContent();
        }

        [HttpPost("{id:int}/series-update")]
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

            if (!CanManageReservation(baseReservation, currentUserId))
            {
                return Forbid();
            }

            var scope = ReservationRules.NormalizeSeriesScope(request.Scope);
            if (scope == ReservationSeriesScopes.Single || !ReservationRules.IsRecurring(baseReservation))
            {
                return await PutReservation(
                    id,
                    request.UpdatedReservation,
                    request.NotifyParticipants,
                    cancellationToken);
            }

            var updatedTemplate = request.UpdatedReservation ?? new ReservationModel();
            ReservationRules.Normalize(updatedTemplate);
            updatedTemplate.UserId = baseReservation.UserId;
            updatedTemplate.Name = baseReservation.Name;
            updatedTemplate.SeriesId = baseReservation.SeriesId;

            var templateValidationError = ReservationRules.Validate(updatedTemplate);
            if (!string.IsNullOrWhiteSpace(templateValidationError))
            {
                return BadRequest(templateValidationError);
            }

            var targets = await _reservationSeriesQueryService.GetSeriesReservationsAsync(
                baseReservation,
                scope,
                cancellationToken,
                asNoTracking: false);
            if (targets.Count == 0)
            {
                return NoContent();
            }

            var targetIds = targets.Select(x => x.Id).ToHashSet();
            var dayOffset = updatedTemplate.Date.Date - baseReservation.Date.Date;
            var operations = new List<(ReservationModel Previous, ReservationModel Current, List<int> PreviousParticipantIds, List<int> CurrentParticipantIds)>();

            foreach (var target in targets)
            {
                var nextDate = target.Date.Date.Add(dayOffset);
                var updatedReservation = ReservationRules.CreateRecurringUpdated(target, updatedTemplate, nextDate);
                var updatedValidationError = ReservationRules.Validate(updatedReservation);
                if (!string.IsNullOrWhiteSpace(updatedValidationError))
                {
                    return BadRequest(updatedValidationError);
                }

                var conflict = await _reservationConflictService.FindFirstConflictAsync(
                    updatedReservation,
                    targetIds,
                    cancellationToken);

                if (conflict != null)
                {
                    return Conflict(_reservationConflictService.BuildConflictMessage(conflict));
                }

                var previousReservation = ReservationRules.Clone(target);
                var previousParticipantIds = ReservationRules.GetNotifiableParticipantIds(previousReservation);
                var currentParticipantIds = ReservationRules.GetNotifiableParticipantIds(updatedReservation);

                ReservationRules.ApplyUpdate(target, updatedReservation);
                operations.Add((previousReservation, ReservationRules.Clone(target), previousParticipantIds, currentParticipantIds));
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

            await _context.SaveChangesAsync(cancellationToken);

            if (operations.Count > 0)
            {
                var representativeOperation = operations
                    .OrderBy(operation => operation.Current.Date)
                    .ThenBy(operation => operation.Current.Id)
                    .First();

                await _reservationChatworkNotificationService.SendReservationSeriesUpdatedAsync(
                    representativeOperation.Previous,
                    representativeOperation.Current,
                    operations.Count,
                    cancellationToken);
            }

            return NoContent();
        }

        private async Task NotifyParticipantsForCreatedReservationAsync(ReservationModel reservation)
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

        private async Task<UserModel?> GetCurrentUserAsync(CancellationToken cancellationToken)
        {
            if (!TryGetCurrentUserId(out var currentUserId))
            {
                return null;
            }

            return await _context.Users.FirstOrDefaultAsync(user => user.Id == currentUserId, cancellationToken);
        }

        private bool TryGetCurrentUserId(out int userId)
        {
            var userIdClaim = User.FindFirstValue(ClaimTypes.NameIdentifier);
            return int.TryParse(userIdClaim, out userId);
        }

        private bool CanManageReservation(ReservationModel reservation, int currentUserId)
        {
            return reservation.UserId == currentUserId || User.IsInRole("Admin");
        }
    }
}

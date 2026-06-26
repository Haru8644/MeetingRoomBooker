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
        private readonly AppDbContext _context;
        private readonly IReservationAccessService _reservationAccessService;
        private readonly IReservationChatworkNotificationService _reservationChatworkNotificationService;
        private readonly IReservationNotificationService _reservationNotificationService;
        private readonly IReservationConflictService _reservationConflictService;
        private readonly IReservationSeriesQueryService _reservationSeriesQueryService;

        public ReservationsController(
            AppDbContext context,
            IReservationAccessService reservationAccessService,
            IReservationChatworkNotificationService reservationChatworkNotificationService,
            IReservationNotificationService reservationNotificationService,
            IReservationConflictService reservationConflictService,
            IReservationSeriesQueryService reservationSeriesQueryService)
        {
            _context = context;
            _reservationAccessService = reservationAccessService;
            _reservationChatworkNotificationService = reservationChatworkNotificationService;
            _reservationNotificationService = reservationNotificationService;
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
            var currentUser = await _reservationAccessService.GetCurrentUserAsync(User, cancellationToken);
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

            await _reservationNotificationService.NotifyParticipantsForCreatedReservationAsync(reservation);
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

            var currentUser = await _reservationAccessService.GetCurrentUserAsync(User, cancellationToken);
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

            await _reservationNotificationService.NotifyParticipantsForCreatedReservationAsync(representativeReservation);
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

            if (!_reservationAccessService.TryGetCurrentUserId(User, out var currentUserId))
            {
                return Unauthorized();
            }

            if (!_reservationAccessService.CanManageReservation(User, reservation, currentUserId))
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

            if (!_reservationAccessService.TryGetCurrentUserId(User, out var currentUserId))
            {
                return Unauthorized();
            }

            if (!_reservationAccessService.CanManageReservation(User, existingReservation, currentUserId))
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
                await _reservationNotificationService.NotifyParticipantsForUpdatedReservationAsync(
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

            if (!_reservationAccessService.TryGetCurrentUserId(User, out var currentUserId))
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
            await _reservationNotificationService.NotifyOrganizerForParticipationChangedAsync(reservation, currentUserId, true, cancellationToken);
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

            if (!_reservationAccessService.TryGetCurrentUserId(User, out var currentUserId))
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
            await _reservationNotificationService.NotifyOrganizerForParticipationChangedAsync(reservation, currentUserId, false, cancellationToken);
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

            if (!_reservationAccessService.TryGetCurrentUserId(User, out var currentUserId))
            {
                return Unauthorized();
            }

            if (!_reservationAccessService.CanManageReservation(User, baseReservation, currentUserId))
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

            if (!_reservationAccessService.TryGetCurrentUserId(User, out var currentUserId))
            {
                return Unauthorized();
            }

            if (!_reservationAccessService.CanManageReservation(User, baseReservation, currentUserId))
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
                    await _reservationNotificationService.NotifyParticipantsForUpdatedReservationAsync(
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

    }
}

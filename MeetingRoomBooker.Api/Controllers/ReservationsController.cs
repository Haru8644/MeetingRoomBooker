using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Controllers
{
    [Route("api/[controller]")]
    [ApiController]
    public class ReservationsController : ControllerBase
    {
        private readonly AppDbContext _context;

        public ReservationsController(AppDbContext context)
        {
            _context = context;
        }

        [HttpGet]
        public async Task<ActionResult<IEnumerable<ReservationModel>>> GetReservations()
        {
            return await _context.Reservations.ToListAsync();
        }

        [HttpPost]
        public async Task<ActionResult<ReservationModel>> PostReservation(ReservationModel reservation)
        {
            _context.Reservations.Add(reservation);
            await _context.SaveChangesAsync();

            if (reservation.ParticipantIds != null && reservation.ParticipantIds.Any())
            {
                var targetUsers = reservation.ParticipantIds
                    .Where(id => id != reservation.UserId)
                    .Distinct()
                    .ToList();

                if (targetUsers.Any())
                {
                    if (IsRecurringReservation(reservation))
                    {
                        await UpsertRecurringReservationNotificationsAsync(reservation, targetUsers);
                    }
                    else
                    {
                        var notifications = new List<NotificationModel>();

                        foreach (var userId in targetUsers)
                        {
                            notifications.Add(new NotificationModel
                            {
                                UserId = userId,
                                Type = "Info",
                                Message = $"{reservation.Name}さんが会議「{reservation.Purpose}」にあなたを招待しました。",
                                TargetDate = reservation.Date,
                                TargetReservationId = reservation.Id,
                                CreatedAt = DateTime.Now,
                                IsRead = false
                            });
                        }

                        if (notifications.Any())
                        {
                            _context.Notifications.AddRange(notifications);
                            await _context.SaveChangesAsync();
                        }
                    }
                }
            }

            return CreatedAtAction("GetReservations", new { id = reservation.Id }, reservation);
        }

        [HttpDelete("{id}")]
        public async Task<IActionResult> DeleteReservation(int id)
        {
            var reservation = await _context.Reservations.FindAsync(id);
            if (reservation == null) return NotFound();

            _context.Reservations.Remove(reservation);
            await _context.SaveChangesAsync();
            return NoContent();
        }

        [HttpPut("{id}")]
        public async Task<IActionResult> PutReservation(int id, ReservationModel reservation)
        {
            if (id != reservation.Id) return BadRequest();

            _context.Entry(reservation).State = EntityState.Modified;

            try
            {
                await _context.SaveChangesAsync();
            }
            catch (DbUpdateConcurrencyException)
            {
                if (!_context.Reservations.Any(e => e.Id == id)) return NotFound();
                else throw;
            }

            return NoContent();
        }

        private static bool IsRecurringReservation(ReservationModel reservation)
        {
            return !string.IsNullOrWhiteSpace(reservation.RepeatType)
                && reservation.RepeatType != "しない"
                && reservation.RepeatUntil.HasValue;
        }

        private async Task<List<ReservationModel>> GetRecurringSeriesReservationsAsync(ReservationModel reservation)
        {
            var candidates = await _context.Reservations
                .Where(x =>
                    x.UserId == reservation.UserId &&
                    x.Name == reservation.Name &&
                    x.Room == reservation.Room &&
                    x.Type == reservation.Type &&
                    x.Purpose == reservation.Purpose &&
                    x.RepeatType == reservation.RepeatType)
                .ToListAsync();

            return candidates
                .Where(x =>
                    x.RepeatUntil?.Date == reservation.RepeatUntil?.Date &&
                    x.StartTime.TimeOfDay == reservation.StartTime.TimeOfDay &&
                    x.EndTime.TimeOfDay == reservation.EndTime.TimeOfDay)
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .ToList();
        }

        private async Task UpsertRecurringReservationNotificationsAsync(
            ReservationModel reservation,
            List<int> targetUsers)
        {
            var seriesReservations = await GetRecurringSeriesReservationsAsync(reservation);
            var firstReservation = seriesReservations.FirstOrDefault();

            if (firstReservation == null) return;

            var count = seriesReservations.Count;
            var message = $"{reservation.Name}さんが繰り返し予約「{reservation.Purpose}」にあなたを招待しました。({count}件)";

            foreach (var userId in targetUsers)
            {
                var existing = await _context.Notifications.FirstOrDefaultAsync(n =>
                    n.UserId == userId &&
                    n.Type == "Info" &&
                    n.TargetReservationId == firstReservation.Id);

                if (existing == null)
                {
                    _context.Notifications.Add(new NotificationModel
                    {
                        UserId = userId,
                        Type = "Info",
                        Message = message,
                        TargetDate = firstReservation.Date,
                        TargetReservationId = firstReservation.Id,
                        CreatedAt = DateTime.Now,
                        IsRead = false
                    });

                    continue;
                }

                existing.Message = message;
                existing.TargetDate = firstReservation.Date;
                existing.IsRead = false;
            }

            await _context.SaveChangesAsync();
        }
    }
}
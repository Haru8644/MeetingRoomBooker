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
    }
}
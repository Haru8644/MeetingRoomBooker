using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Controllers
{
    [Route("api/[controller]")]
    [ApiController]
    public class UsersController : ControllerBase
    {
        private static readonly string ProtectedAdminEmail = "haruki_sasuke@icloud.com";
        private readonly AppDbContext _context;

        public UsersController(AppDbContext context)
        {
            _context = context;
        }

        [HttpGet]
        public async Task<ActionResult<IEnumerable<UserModel>>> GetUsers()
        {
            return await _context.Users.ToListAsync();
        }

        [HttpPost("register")]
        public async Task<ActionResult<UserModel>> Register(UserModel user)
        {
            if (await _context.Users.AnyAsync(u => u.Email == user.Email))
            {
                return Conflict("Email already exists");
            }

            _context.Users.Add(user);
            await _context.SaveChangesAsync();

            return CreatedAtAction(nameof(GetUsers), new { id = user.Id }, user);
        }

        [HttpPost("login")]
        public async Task<ActionResult<UserModel>> Login([FromBody] LoginRequest loginInfo)
        {
            var user = await _context.Users
                .FirstOrDefaultAsync(u => u.Email == loginInfo.Email && u.Password == loginInfo.Password);

            if (user == null)
            {
                return Unauthorized("Invalid email or password");
            }

            return user;
        }

        [HttpDelete("{id:int}")]
        public async Task<IActionResult> DeleteUser(int id)
        {
            var user = await _context.Users.FirstOrDefaultAsync(u => u.Id == id);
            if (user == null)
            {
                return NotFound();
            }

            if (IsProtectedAdmin(user))
            {
                return BadRequest("The admin user cannot be deleted.");
            }

            await using var transaction = await _context.Database.BeginTransactionAsync();

            var allReservations = await _context.Reservations.ToListAsync();

            var ownedReservations = allReservations
                .Where(r => r.UserId == id)
                .ToList();

            var ownedReservationIds = ownedReservations
                .Select(r => r.Id)
                .ToList();

            var participantReservations = allReservations
                .Where(r =>
                    r.UserId != id &&
                    r.ParticipantIds != null &&
                    r.ParticipantIds.Contains(id))
                .ToList();

            foreach (var reservation in participantReservations)
            {
                reservation.ParticipantIds = reservation.ParticipantIds
                    .Where(participantId => participantId != id)
                    .Distinct()
                    .ToList();

                reservation.ParticipantCount = reservation.ParticipantIds.Count;
            }

            if (ownedReservationIds.Count > 0)
            {
                var reservationNotifications = await _context.Notifications
                    .Where(n =>
                        n.TargetReservationId.HasValue &&
                        ownedReservationIds.Contains(n.TargetReservationId.Value))
                    .ToListAsync();

                if (reservationNotifications.Count > 0)
                {
                    _context.Notifications.RemoveRange(reservationNotifications);
                }
            }

            var userNotifications = await _context.Notifications
                .Where(n => n.UserId == id)
                .ToListAsync();

            if (userNotifications.Count > 0)
            {
                _context.Notifications.RemoveRange(userNotifications);
            }

            if (ownedReservations.Count > 0)
            {
                _context.Reservations.RemoveRange(ownedReservations);
            }

            _context.Users.Remove(user);

            await _context.SaveChangesAsync();
            await transaction.CommitAsync();

            return NoContent();
        }

        private static bool IsProtectedAdmin(UserModel user)
        {
            return user.IsAdmin
                   || string.Equals(user.Email, ProtectedAdminEmail, StringComparison.OrdinalIgnoreCase)
                   || string.Equals(user.Name, "Haru", StringComparison.OrdinalIgnoreCase);
        }
    }

    public class LoginRequest
    {
        public string Email { get; set; } = string.Empty;
        public string Password { get; set; } = string.Empty;
    }
}

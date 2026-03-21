using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Controllers
{
    [Route("api/[controller]")]
    [ApiController]
    public class UsersController : ControllerBase
    {
        private const int ChatworkAccountIdMaxLength = 100;
        private static readonly string ProtectedAdminEmail = "haruki_sasuke@icloud.com";
        private readonly AppDbContext _context;

        public UsersController(AppDbContext context)
        {
            _context = context;
        }

        [HttpGet]
        public async Task<ActionResult<IEnumerable<UserModel>>> GetUsers()
        {
            return await _context.Users
                .OrderBy(user => user.Id)
                .ToListAsync();
        }

        [HttpPost("register")]
        public async Task<ActionResult<UserModel>> Register(UserModel user)
        {
            user.Name = user.Name?.Trim() ?? string.Empty;
            user.Email = user.Email?.Trim() ?? string.Empty;
            user.Password = user.Password ?? string.Empty;
            user.ChatworkAccountId = NormalizeChatworkAccountId(user.ChatworkAccountId);

            if (string.IsNullOrWhiteSpace(user.Name)
                || string.IsNullOrWhiteSpace(user.Email)
                || string.IsNullOrWhiteSpace(user.Password))
            {
                return BadRequest("Name, email, and password are required.");
            }

            if (user.ChatworkAccountId?.Length > ChatworkAccountIdMaxLength)
            {
                return BadRequest($"Chatwork account ID must be {ChatworkAccountIdMaxLength} characters or less.");
            }

            if (await _context.Users.AnyAsync(u => u.Email == user.Email))
            {
                return Conflict("Email already exists");
            }

            if (string.IsNullOrWhiteSpace(user.AvatarColor))
            {
                user.AvatarColor = "#58a6ff";
            }

            _context.Users.Add(user);
            await _context.SaveChangesAsync();

            return CreatedAtAction(nameof(GetUsers), new { id = user.Id }, user);
        }

        [HttpPost("login")]
        public async Task<ActionResult<UserModel>> Login([FromBody] LoginRequest loginInfo)
        {
            var email = loginInfo.Email?.Trim() ?? string.Empty;
            var password = loginInfo.Password ?? string.Empty;

            var user = await _context.Users
                .FirstOrDefaultAsync(u => u.Email == email && u.Password == password);

            if (user == null)
            {
                return Unauthorized("Invalid email or password");
            }

            return user;
        }

        [HttpPut("{id:int}/chatwork-account")]
        public async Task<IActionResult> UpdateChatworkAccountId(
            int id,
            [FromBody] UpdateUserChatworkAccountRequest request)
        {
            var user = await _context.Users.FirstOrDefaultAsync(u => u.Id == id);
            if (user == null)
            {
                return NotFound();
            }

            var normalizedAccountId = NormalizeChatworkAccountId(request.ChatworkAccountId);
            if (normalizedAccountId?.Length > ChatworkAccountIdMaxLength)
            {
                return BadRequest($"Chatwork account ID must be {ChatworkAccountIdMaxLength} characters or less.");
            }

            user.ChatworkAccountId = normalizedAccountId;
            await _context.SaveChangesAsync();

            return NoContent();
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

        private static string? NormalizeChatworkAccountId(string? chatworkAccountId)
        {
            return string.IsNullOrWhiteSpace(chatworkAccountId)
                ? null
                : chatworkAccountId.Trim();
        }

        private static bool IsProtectedAdmin(UserModel user)
        {
            return user.IsAdmin
                   || string.Equals(user.Email, ProtectedAdminEmail, StringComparison.OrdinalIgnoreCase)
                   || string.Equals(user.Name, "Haru", StringComparison.OrdinalIgnoreCase);
        }

        public sealed class LoginRequest
        {
            public string Email { get; set; } = string.Empty;
            public string Password { get; set; } = string.Empty;
        }

        public sealed class UpdateUserChatworkAccountRequest
        {
            public string? ChatworkAccountId { get; set; }
        }
    }
}
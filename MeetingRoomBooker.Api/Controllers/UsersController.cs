using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Identity;
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
        private static readonly PasswordHasher<UserModel> PasswordHasher = new();
        private readonly AppDbContext _context;

        public UsersController(AppDbContext context)
        {
            _context = context;
        }

        [Authorize]
        [HttpGet]
        public async Task<ActionResult<IEnumerable<UserModel>>> GetUsers()
        {
            return await _context.Users
                .OrderBy(user => user.Id)
                .ToListAsync();
        }

        [AllowAnonymous]
        [HttpPost("register")]
        public async Task<ActionResult<UserModel>> Register(RegisterUserRequest request)
        {
            var name = request.Name?.Trim() ?? string.Empty;
            var email = request.Email?.Trim() ?? string.Empty;
            var password = request.Password ?? string.Empty;
            var chatworkAccountId = NormalizeChatworkAccountId(request.ChatworkAccountId);
            var hasExistingUsers = await _context.Users.AnyAsync();

            if (hasExistingUsers && (!User.Identity?.IsAuthenticated ?? true || !User.IsInRole("Admin")))
            {
                return Forbid();
            }

            if (string.IsNullOrWhiteSpace(name)
                || string.IsNullOrWhiteSpace(email)
                || string.IsNullOrWhiteSpace(password))
            {
                return BadRequest("Name, email, and password are required.");
            }

            if (chatworkAccountId?.Length > ChatworkAccountIdMaxLength)
            {
                return BadRequest($"Chatwork account ID must be {ChatworkAccountIdMaxLength} characters or less.");
            }

            if (await _context.Users.AnyAsync(u => u.Email == email))
            {
                return Conflict("Email already exists.");
            }

            var user = new UserModel
            {
                Name = name,
                Email = email,
                PasswordHash = string.Empty,
                AvatarColor = string.IsNullOrWhiteSpace(request.AvatarColor) ? "#58a6ff" : request.AvatarColor,
                ChatworkAccountId = chatworkAccountId,
                IsAdmin = !hasExistingUsers
            };

            user.PasswordHash = PasswordHasher.HashPassword(user, password);

            _context.Users.Add(user);
            await _context.SaveChangesAsync();

            return CreatedAtAction(nameof(GetUsers), new { id = user.Id }, user);
        }

        [Authorize(Policy = "AdminOnly")]
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

        [Authorize(Policy = "AdminOnly")]
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

        public sealed class UpdateUserChatworkAccountRequest
        {
            public string? ChatworkAccountId { get; set; }
        }
    }
}
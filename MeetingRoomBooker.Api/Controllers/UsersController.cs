using System.Security.Claims;
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
    public sealed class UsersController : ControllerBase
    {
        private const int ChatworkIdMaxLength = 100;
        private static readonly PasswordHasher<UserModel> PasswordHasher = new();
        private readonly AppDbContext _context;

        public UsersController(AppDbContext context)
        {
            _context = context;
        }

        [Authorize(Policy = "AdminOnly")]
        [HttpGet]
        public async Task<ActionResult<IEnumerable<UserModel>>> GetUsers()
        {
            return await _context.Users
                .AsNoTracking()
                .OrderBy(user => user.Id)
                .ToListAsync();
        }

        [Authorize]
        [HttpGet("participants")]
        public async Task<ActionResult<IEnumerable<ParticipantUserModel>>> GetParticipantUsers()
        {
            return await _context.Users
                .AsNoTracking()
                .OrderBy(user => user.Id)
                .Select(user => new ParticipantUserModel
                {
                    Id = user.Id,
                    Name = user.Name,
                    AvatarColor = user.AvatarColor
                })
                .ToListAsync();
        }

        [AllowAnonymous]
        [HttpPost("register")]
        public async Task<ActionResult<UserModel>> Register(RegisterUserRequest request)
        {
            var name = request.Name?.Trim() ?? string.Empty;
            var email = NormalizeEmail(request.Email);
            var password = request.Password ?? string.Empty;
            var chatworkAccountId = NormalizeChatworkId(request.ChatworkAccountId);
            var chatworkDirectRoomId = NormalizeChatworkId(request.ChatworkDirectRoomId);
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

            if (chatworkAccountId?.Length > ChatworkIdMaxLength)
            {
                return BadRequest($"Chatwork account ID must be {ChatworkIdMaxLength} characters or less.");
            }

            if (chatworkDirectRoomId?.Length > ChatworkIdMaxLength)
            {
                return BadRequest($"Chatwork direct room ID must be {ChatworkIdMaxLength} characters or less.");
            }

            if (await _context.Users.AnyAsync(u => u.Email == email))
            {
                return Conflict("Email already exists.");
            }

            var user = new UserModel
            {
                Name = name,
                Email = email,
                Password = string.Empty,
                PasswordHash = null,
                AvatarColor = string.IsNullOrWhiteSpace(request.AvatarColor) ? "#58a6ff" : request.AvatarColor,
                ChatworkAccountId = chatworkAccountId,
                ChatworkDirectRoomId = chatworkDirectRoomId,
                IsAdmin = !hasExistingUsers
            };

            user.PasswordHash = PasswordHasher.HashPassword(user, password);

            _context.Users.Add(user);
            await _context.SaveChangesAsync();

            return CreatedAtAction(nameof(GetUsers), new { id = user.Id }, user);
        }

        [Authorize(Policy = "AdminOnly")]
        [HttpPut("{id:int}/name")]
        public async Task<IActionResult> UpdateUserName(
            int id,
            [FromBody] UpdateUserNameRequest request)
        {
            var user = await _context.Users.FirstOrDefaultAsync(u => u.Id == id);
            if (user == null)
            {
                return NotFound();
            }

            var normalizedName = request.Name?.Trim() ?? string.Empty;
            if (string.IsNullOrWhiteSpace(normalizedName))
            {
                return BadRequest("Name is required.");
            }

            if (normalizedName.Length > 100)
            {
                return BadRequest("Name must be 100 characters or less.");
            }

            user.Name = normalizedName;

            var ownedReservations = await _context.Reservations
                .Where(r => r.UserId == id)
                .ToListAsync();

            foreach (var reservation in ownedReservations)
            {
                reservation.Name = normalizedName;
            }

            await _context.SaveChangesAsync();

            return NoContent();
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

            var normalizedAccountId = NormalizeChatworkId(request.ChatworkAccountId);
            if (normalizedAccountId?.Length > ChatworkIdMaxLength)
            {
                return BadRequest($"Chatwork account ID must be {ChatworkIdMaxLength} characters or less.");
            }

            user.ChatworkAccountId = normalizedAccountId;
            await _context.SaveChangesAsync();

            return NoContent();
        }

        [Authorize(Policy = "AdminOnly")]
        [HttpPut("{id:int}/chatwork-direct-room")]
        public async Task<IActionResult> UpdateChatworkDirectRoomId(
            int id,
            [FromBody] UpdateUserChatworkDirectRoomRequest request)
        {
            var user = await _context.Users.FirstOrDefaultAsync(u => u.Id == id);
            if (user == null)
            {
                return NotFound();
            }

            var normalizedDirectRoomId = NormalizeChatworkId(request.ChatworkDirectRoomId);
            if (normalizedDirectRoomId?.Length > ChatworkIdMaxLength)
            {
                return BadRequest($"Chatwork direct room ID must be {ChatworkIdMaxLength} characters or less.");
            }

            user.ChatworkDirectRoomId = normalizedDirectRoomId;
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

            if (user.IsAdmin)
            {
                return BadRequest("管理者ユーザーは削除できません。");
            }

            var currentUserId = TryGetCurrentUserId(out var parsedUserId)
                ? parsedUserId
                : 0;

            if (currentUserId == id)
            {
                return BadRequest("自分自身は削除できません。");
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

        private static string NormalizeEmail(string? email)
        {
            return (email ?? string.Empty).Trim().ToLowerInvariant();
        }

        private static string? NormalizeChatworkId(string? chatworkId)
        {
            return string.IsNullOrWhiteSpace(chatworkId)
                ? null
                : chatworkId.Trim();
        }

        private bool TryGetCurrentUserId(out int userId)
        {
            var userIdClaim = User.FindFirstValue(ClaimTypes.NameIdentifier);
            return int.TryParse(userIdClaim, out userId);
        }

        public sealed class UpdateUserNameRequest
        {
            public string Name { get; set; } = string.Empty;
        }

        public sealed class UpdateUserChatworkAccountRequest
        {
            public string? ChatworkAccountId { get; set; }
        }

        public sealed class UpdateUserChatworkDirectRoomRequest
        {
            public string? ChatworkDirectRoomId { get; set; }
        }
    }
}
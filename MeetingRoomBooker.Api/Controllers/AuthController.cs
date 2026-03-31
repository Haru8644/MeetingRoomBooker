using System.Security.Claims;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Controllers
{
    [ApiController]
    [Route("api/[controller]")]
    public sealed class AuthController : ControllerBase
    {
        private static readonly PasswordHasher<UserModel> PasswordHasher = new();
        private static readonly string ProtectedAdminEmail = "haruki_sasuke@icloud.com";
        private readonly AppDbContext _context;

        public AuthController(AppDbContext context)
        {
            _context = context;
        }

        [AllowAnonymous]
        [HttpPost("login")]
        public async Task<ActionResult<UserModel>> Login([FromBody] LoginRequest request)
        {
            var email = request.Email?.Trim() ?? string.Empty;
            var password = request.Password ?? string.Empty;

            if (string.IsNullOrWhiteSpace(email) || string.IsNullOrWhiteSpace(password))
            {
                return Unauthorized("Invalid email or password.");
            }

            var user = await _context.Users.FirstOrDefaultAsync(x => x.Email == email);
            if (user == null)
            {
                return Unauthorized("Invalid email or password.");
            }

            var isAuthenticated = false;
            var shouldSave = false;

            if (!string.IsNullOrWhiteSpace(user.PasswordHash))
            {
                var verificationResult = PasswordHasher.VerifyHashedPassword(
                    user,
                    user.PasswordHash,
                    password);

                if (verificationResult != PasswordVerificationResult.Failed)
                {
                    isAuthenticated = true;

                    if (verificationResult == PasswordVerificationResult.SuccessRehashNeeded)
                    {
                        user.PasswordHash = PasswordHasher.HashPassword(user, password);
                        shouldSave = true;
                    }
                }
            }

            if (!isAuthenticated && !string.IsNullOrWhiteSpace(user.Password) && user.Password == password)
            {
                isAuthenticated = true;
                user.PasswordHash = PasswordHasher.HashPassword(user, password);
                shouldSave = true;
            }

            if (!isAuthenticated)
            {
                return Unauthorized("Invalid email or password.");
            }

            var isAdmin = ShouldTreatAsAdmin(user);
            if (isAdmin && !user.IsAdmin)
            {
                user.IsAdmin = true;
                shouldSave = true;
            }

            if (shouldSave)
            {
                await _context.SaveChangesAsync();
            }

            var claims = new List<Claim>
            {
                new(ClaimTypes.NameIdentifier, user.Id.ToString()),
                new(ClaimTypes.Name, user.Name),
                new(ClaimTypes.Email, user.Email)
            };

            if (isAdmin)
            {
                claims.Add(new Claim(ClaimTypes.Role, "Admin"));
            }

            var principal = new ClaimsPrincipal(
                new ClaimsIdentity(claims, CookieAuthenticationDefaults.AuthenticationScheme));

            var properties = new AuthenticationProperties
            {
                IsPersistent = request.RememberMe,
                AllowRefresh = true
            };

            if (request.RememberMe)
            {
                properties.ExpiresUtc = DateTimeOffset.UtcNow.AddDays(14);
            }

            await HttpContext.SignInAsync(
                CookieAuthenticationDefaults.AuthenticationScheme,
                principal,
                properties);

            return Ok(user);
        }

        [Authorize]
        [HttpPost("logout")]
        public async Task<IActionResult> Logout()
        {
            await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);
            return NoContent();
        }

        [Authorize]
        [HttpGet("me")]
        public async Task<ActionResult<UserModel>> Me()
        {
            var userIdClaim = User.FindFirstValue(ClaimTypes.NameIdentifier);
            if (!int.TryParse(userIdClaim, out var userId))
            {
                await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);
                return Unauthorized();
            }

            var user = await _context.Users.FirstOrDefaultAsync(x => x.Id == userId);
            if (user == null)
            {
                await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);
                return Unauthorized();
            }

            if (ShouldTreatAsAdmin(user) && !user.IsAdmin)
            {
                user.IsAdmin = true;
                await _context.SaveChangesAsync();
            }

            return Ok(user);
        }

        private static bool ShouldTreatAsAdmin(UserModel user)
        {
            return user.IsAdmin
                   || string.Equals(user.Email, ProtectedAdminEmail, StringComparison.OrdinalIgnoreCase)
                   || string.Equals(user.Name, "Haru", StringComparison.OrdinalIgnoreCase);
        }
    }
}
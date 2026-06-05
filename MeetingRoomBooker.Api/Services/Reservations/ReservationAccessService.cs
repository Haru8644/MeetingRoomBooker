using System.Security.Claims;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Reservations;

public interface IReservationAccessService
{
    Task<UserModel?> GetCurrentUserAsync(
        ClaimsPrincipal user,
        CancellationToken cancellationToken);

    bool TryGetCurrentUserId(
        ClaimsPrincipal user,
        out int userId);

    bool CanManageReservation(
        ClaimsPrincipal user,
        ReservationModel reservation,
        int currentUserId);
}

public sealed class ReservationAccessService : IReservationAccessService
{
    private readonly AppDbContext _context;

    public ReservationAccessService(AppDbContext context)
    {
        _context = context;
    }

    public async Task<UserModel?> GetCurrentUserAsync(
        ClaimsPrincipal user,
        CancellationToken cancellationToken)
    {
        if (!TryGetCurrentUserId(user, out var currentUserId))
        {
            return null;
        }

        return await _context.Users
            .FirstOrDefaultAsync(x => x.Id == currentUserId, cancellationToken);
    }

    public bool TryGetCurrentUserId(
        ClaimsPrincipal user,
        out int userId)
    {
        var userIdClaim = user.FindFirstValue(ClaimTypes.NameIdentifier);
        return int.TryParse(userIdClaim, out userId);
    }

    public bool CanManageReservation(
        ClaimsPrincipal user,
        ReservationModel reservation,
        int currentUserId)
    {
        return reservation.UserId == currentUserId || user.IsInRole("Admin");
    }
}

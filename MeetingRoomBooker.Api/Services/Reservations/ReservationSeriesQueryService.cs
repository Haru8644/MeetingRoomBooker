using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Reservations;

public interface IReservationSeriesQueryService
{
    Task<List<ReservationModel>> GetSeriesReservationsAsync(
        ReservationModel reservation,
        string scope,
        CancellationToken cancellationToken,
        bool asNoTracking = true);
}

public sealed class ReservationSeriesQueryService : IReservationSeriesQueryService
{
    private readonly AppDbContext _context;

    public ReservationSeriesQueryService(AppDbContext context)
    {
        _context = context;
    }

    public async Task<List<ReservationModel>> GetSeriesReservationsAsync(
        ReservationModel reservation,
        string scope,
        CancellationToken cancellationToken,
        bool asNoTracking = true)
    {
        IQueryable<ReservationModel> query = _context.Reservations;

        if (asNoTracking)
        {
            query = query.AsNoTracking();
        }

        var normalizedSeriesId = reservation.SeriesId?.Trim();
        List<ReservationModel> matches;

        if (!string.IsNullOrWhiteSpace(normalizedSeriesId))
        {
            matches = await query
                .Where(x =>
                    x.UserId == reservation.UserId &&
                    x.SeriesId == normalizedSeriesId)
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .ToListAsync(cancellationToken);
        }
        else
        {
            var candidates = await query
                .Where(x =>
                    x.UserId == reservation.UserId &&
                    x.Name == reservation.Name &&
                    x.Room == reservation.Room &&
                    x.Type == reservation.Type &&
                    x.Purpose == reservation.Purpose &&
                    x.RepeatType == reservation.RepeatType)
                .ToListAsync(cancellationToken);

            matches = candidates
                .Where(x =>
                    x.RepeatUntil?.Date == reservation.RepeatUntil?.Date &&
                    x.StartTime.TimeOfDay == reservation.StartTime.TimeOfDay &&
                    x.EndTime.TimeOfDay == reservation.EndTime.TimeOfDay)
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .ToList();
        }

        return scope == ReservationSeriesScopes.Following
            ? matches.Where(x => x.Date.Date >= reservation.Date.Date).ToList()
            : matches;
    }
}

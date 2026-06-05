using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Reservations;

public interface IReservationConflictService
{
    Task<ReservationModel?> FindFirstConflictAsync(
        ReservationModel reservation,
        IEnumerable<int>? excludedReservationIds = null,
        CancellationToken cancellationToken = default);

    Task<List<ReservationModel>> FindConflictsAsync(
        ReservationModel reservation,
        IEnumerable<int>? excludedReservationIds = null,
        CancellationToken cancellationToken = default);

    string BuildConflictMessage(ReservationModel conflict);
}

public sealed class ReservationConflictService : IReservationConflictService
{
    private readonly AppDbContext _context;

    public ReservationConflictService(AppDbContext context)
    {
        _context = context;
    }

    public async Task<ReservationModel?> FindFirstConflictAsync(
        ReservationModel reservation,
        IEnumerable<int>? excludedReservationIds = null,
        CancellationToken cancellationToken = default)
    {
        var conflicts = await FindConflictsAsync(
            reservation,
            excludedReservationIds,
            cancellationToken);

        return conflicts.FirstOrDefault();
    }

    public async Task<List<ReservationModel>> FindConflictsAsync(
        ReservationModel reservation,
        IEnumerable<int>? excludedReservationIds = null,
        CancellationToken cancellationToken = default)
    {
        var excludedIds = excludedReservationIds?
            .Where(id => id > 0)
            .Distinct()
            .ToHashSet() ?? new HashSet<int>();

        return await _context.Reservations
            .AsNoTracking()
            .Where(x =>
                !excludedIds.Contains(x.Id) &&
                x.Room == reservation.Room &&
                x.Date.Date == reservation.Date.Date &&
                x.StartTime < reservation.EndTime &&
                x.EndTime > reservation.StartTime)
            .OrderBy(x => x.Date)
            .ThenBy(x => x.StartTime)
            .ToListAsync(cancellationToken);
    }

    public string BuildConflictMessage(ReservationModel conflict)
    {
        var label = string.IsNullOrWhiteSpace(conflict.Purpose) ? conflict.Name : conflict.Purpose;
        return $"この時間帯には既に予約があります。会議室: {conflict.Room} / 利用日: {conflict.Date:yyyy/MM/dd} / 時間: {conflict.StartTime:HH:mm}〜{conflict.EndTime:HH:mm} / 予約: {label}";
    }
}

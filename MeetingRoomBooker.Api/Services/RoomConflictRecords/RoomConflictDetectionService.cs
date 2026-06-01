using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.Reservations;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.RoomConflictRecords;

public sealed class RoomConflictDetectionService : IRoomConflictDetectionService
{
    private readonly AppDbContext _context;
    private readonly ILogger<RoomConflictDetectionService> _logger;

    public RoomConflictDetectionService(
        AppDbContext context,
        ILogger<RoomConflictDetectionService> logger)
    {
        _context = context;
        _logger = logger;
    }

    public async Task<int> DetectUnresolvedOverlapsAsync(
        DateTime now,
        TimeSpan lookbackWindow,
        CancellationToken cancellationToken)
    {
        var detectFrom = now - lookbackWindow;

        var reservations = await _context.Reservations
            .AsNoTracking()
            .Where(reservation =>
                reservation.EndTime > detectFrom &&
                reservation.StartTime <= now)
            .OrderBy(reservation => reservation.StartTime)
            .ThenBy(reservation => reservation.Id)
            .ToListAsync(cancellationToken);

        if (reservations.Count < 2)
        {
            return 0;
        }

        var recordsToAdd = new List<RoomConflictRecord>();
        var detectionKeys = new HashSet<string>();

        for (var i = 0; i < reservations.Count; i++)
        {
            for (var j = i + 1; j < reservations.Count; j++)
            {
                var left = reservations[i];
                var right = reservations[j];

                if (!ReservationOverlapChecker.IsConflicting(left, right))
                {
                    continue;
                }

                var overlapStart = left.StartTime > right.StartTime
                    ? left.StartTime
                    : right.StartTime;

                var overlapEnd = left.EndTime < right.EndTime
                    ? left.EndTime
                    : right.EndTime;

                if (overlapStart < detectFrom || overlapStart > now)
                {
                    continue;
                }

                var detectionKey = BuildDetectionKey(
                    left.Id,
                    right.Id,
                    left.Room,
                    left.Date.Date,
                    overlapStart,
                    overlapEnd);

                if (!detectionKeys.Add(detectionKey))
                {
                    continue;
                }

                var alreadyExists = await _context.RoomConflictRecords
                    .AsNoTracking()
                    .AnyAsync(
                        record => record.DetectionKey == detectionKey,
                        cancellationToken);

                if (alreadyExists)
                {
                    continue;
                }

                recordsToAdd.Add(new RoomConflictRecord
                {
                    Type = ConflictRecordType.UnresolvedReservationOverlap,
                    Status = ConflictStatus.Detected,
                    OccurredAt = overlapStart,
                    RoomName = left.Room,
                    ReservationIdA = Math.Min(left.Id, right.Id),
                    ReservationIdB = Math.Max(left.Id, right.Id),
                    Impact = ConflictImpact.Medium,
                    Cause = ConflictCause.Unknown,
                    Description = "同じ会議室の予約が未解消のまま重複時間帯を迎えました。",
                    Resolution = string.Empty,
                    DetectionKey = detectionKey,
                    ReportedByUserId = null,
                    CreatedAt = now
                });
            }
        }

        if (recordsToAdd.Count == 0)
        {
            return 0;
        }

        _context.RoomConflictRecords.AddRange(recordsToAdd);
        await _context.SaveChangesAsync(cancellationToken);

        _logger.LogInformation(
            "Created {Count} unresolved reservation overlap records.",
            recordsToAdd.Count);

        return recordsToAdd.Count;
    }

    private static string BuildDetectionKey(
        int reservationIdA,
        int reservationIdB,
        string roomName,
        DateTime date,
        DateTime overlapStart,
        DateTime overlapEnd)
    {
        var firstReservationId = Math.Min(reservationIdA, reservationIdB);
        var secondReservationId = Math.Max(reservationIdA, reservationIdB);

        return string.Join(
            "|",
            roomName.Trim(),
            date.ToString("yyyyMMdd"),
            firstReservationId,
            secondReservationId,
            overlapStart.ToString("yyyyMMddHHmmss"),
            overlapEnd.ToString("yyyyMMddHHmmss"));
    }
}

using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public sealed class WorkScheduleParticipantConflictService : IWorkScheduleParticipantConflictService
{
    private readonly AppDbContext _context;

    public WorkScheduleParticipantConflictService(AppDbContext context)
    {
        _context = context;
    }

    public async Task<IReadOnlyList<WorkScheduleParticipantConflict>> FindReservationExternalAppointmentConflictsAsync(
        ReservationModel reservation,
        CancellationToken cancellationToken)
    {
        var participantIds = GetReservationParticipantIds(reservation);
        if (participantIds.Count == 0)
        {
            return Array.Empty<WorkScheduleParticipantConflict>();
        }

        var startTime = reservation.Date.Date + reservation.StartTime.TimeOfDay;
        var endTime = reservation.Date.Date + reservation.EndTime.TimeOfDay;
        if (startTime >= endTime)
        {
            return Array.Empty<WorkScheduleParticipantConflict>();
        }

        var candidateEntries = await _context.WorkScheduleEntries
            .AsNoTracking()
            .Where(entry =>
                entry.Type == WorkScheduleEntryType.ExternalAppointment &&
                entry.Date == reservation.Date.Date)
            .ToListAsync(cancellationToken);

        var conflicts = candidateEntries
            .Where(entry => IsOverlapping(startTime, endTime, entry.StartTime, entry.EndTime))
            .SelectMany(entry => BuildEntryConflicts(entry, participantIds))
            .ToList();

        return await AttachParticipantNamesAsync(conflicts, cancellationToken);
    }

    public async Task<IReadOnlyList<WorkScheduleParticipantConflict>> FindExternalAppointmentConflictsAsync(
        WorkScheduleEntryModel entry,
        CancellationToken cancellationToken)
    {
        if (entry.Type != WorkScheduleEntryType.ExternalAppointment)
        {
            return Array.Empty<WorkScheduleParticipantConflict>();
        }

        var participantIds = GetWorkScheduleParticipantIds(entry);
        if (participantIds.Count == 0 || !entry.StartTime.HasValue || !entry.EndTime.HasValue)
        {
            return Array.Empty<WorkScheduleParticipantConflict>();
        }

        if (entry.StartTime.Value >= entry.EndTime.Value)
        {
            return Array.Empty<WorkScheduleParticipantConflict>();
        }

        var reservationCandidates = await _context.Reservations
            .AsNoTracking()
            .Where(reservation => reservation.Date == entry.Date.Date)
            .ToListAsync(cancellationToken);

        var workScheduleCandidates = await _context.WorkScheduleEntries
            .AsNoTracking()
            .Where(candidate =>
                candidate.Type == WorkScheduleEntryType.ExternalAppointment &&
                candidate.Date == entry.Date.Date &&
                candidate.Id != entry.Id)
            .ToListAsync(cancellationToken);

        var conflicts = new List<WorkScheduleParticipantConflict>();

        conflicts.AddRange(reservationCandidates
            .Where(reservation => IsOverlapping(entry.StartTime.Value, entry.EndTime.Value, reservation.StartTime, reservation.EndTime))
            .SelectMany(reservation => BuildReservationConflicts(reservation, participantIds)));

        conflicts.AddRange(workScheduleCandidates
            .Where(candidate => IsOverlapping(entry.StartTime.Value, entry.EndTime.Value, candidate.StartTime, candidate.EndTime))
            .SelectMany(candidate => BuildEntryConflicts(candidate, participantIds)));

        return await AttachParticipantNamesAsync(conflicts, cancellationToken);
    }

    private async Task<IReadOnlyList<WorkScheduleParticipantConflict>> AttachParticipantNamesAsync(
        IReadOnlyCollection<WorkScheduleParticipantConflict> conflicts,
        CancellationToken cancellationToken)
    {
        if (conflicts.Count == 0)
        {
            return Array.Empty<WorkScheduleParticipantConflict>();
        }

        var userIds = conflicts
            .Select(conflict => conflict.ParticipantUserId)
            .Where(id => id > 0)
            .Distinct()
            .ToList();

        var usersById = await _context.Users
            .AsNoTracking()
            .Where(user => userIds.Contains(user.Id))
            .ToDictionaryAsync(user => user.Id, user => user.Name, cancellationToken);

        return conflicts
            .Select(conflict => conflict with
            {
                ParticipantName = usersById.TryGetValue(conflict.ParticipantUserId, out var name)
                    ? name
                    : $"ユーザー{conflict.ParticipantUserId}"
            })
            .OrderBy(conflict => conflict.Date)
            .ThenBy(conflict => conflict.StartTime ?? conflict.Date)
            .ThenBy(conflict => conflict.ParticipantName)
            .ThenBy(conflict => conflict.SourceType)
            .ThenBy(conflict => conflict.SourceId)
            .ToList();
    }

    private static IEnumerable<WorkScheduleParticipantConflict> BuildReservationConflicts(
        ReservationModel reservation,
        IReadOnlyCollection<int> targetParticipantIds)
    {
        return GetReservationParticipantIds(reservation)
            .Intersect(targetParticipantIds)
            .Select(participantId => new WorkScheduleParticipantConflict(
                participantId,
                string.Empty,
                "会議室予約",
                reservation.Id,
                GetReservationLabel(reservation),
                reservation.Date.Date,
                reservation.StartTime,
                reservation.EndTime));
    }

    private static IEnumerable<WorkScheduleParticipantConflict> BuildEntryConflicts(
        WorkScheduleEntry entry,
        IReadOnlyCollection<int> targetParticipantIds)
    {
        return GetWorkScheduleParticipantIds(entry)
            .Intersect(targetParticipantIds)
            .Select(participantId => new WorkScheduleParticipantConflict(
                participantId,
                string.Empty,
                "社外予定",
                entry.Id,
                entry.Title,
                entry.Date.Date,
                entry.StartTime,
                entry.EndTime));
    }

    private static List<int> GetReservationParticipantIds(ReservationModel reservation)
    {
        return (reservation.ParticipantIds ?? new List<int>())
            .Append(reservation.UserId)
            .Where(id => id > 0)
            .Distinct()
            .ToList();
    }

    private static List<int> GetWorkScheduleParticipantIds(WorkScheduleEntryModel entry)
    {
        return (entry.ParticipantIds ?? new List<int>())
            .Where(id => id > 0)
            .Distinct()
            .ToList();
    }

    private static List<int> GetWorkScheduleParticipantIds(WorkScheduleEntry entry)
    {
        return (entry.ParticipantIds ?? new List<int>())
            .Where(id => id > 0)
            .Distinct()
            .ToList();
    }

    private static bool IsOverlapping(
        DateTime startTime,
        DateTime endTime,
        DateTime? candidateStartTime,
        DateTime? candidateEndTime)
    {
        return candidateStartTime.HasValue &&
               candidateEndTime.HasValue &&
               startTime < candidateEndTime.Value &&
               endTime > candidateStartTime.Value;
    }

    private static string GetReservationLabel(ReservationModel reservation)
    {
        return string.IsNullOrWhiteSpace(reservation.Purpose)
            ? reservation.Name
            : reservation.Purpose;
    }
}

using MeetingRoomBooker.Api.Contracts.RoomConflictRecords;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.RoomConflictRecords;

public sealed class RoomConflictRecordService : IRoomConflictRecordService
{
    private const int RoomNameMaxLength = 100;
    private const int DescriptionMaxLength = 1000;
    private const int ResolutionMaxLength = 1000;

    private readonly AppDbContext _context;

    public RoomConflictRecordService(AppDbContext context)
    {
        _context = context;
    }

    public async Task<IReadOnlyList<RoomConflictRecordResponse>> GetRecordsAsync(
        int currentUserId,
        bool isAdmin)
    {
        var records = await _context.RoomConflictRecords
            .AsNoTracking()
            .OrderByDescending(record => record.OccurredAt)
            .ThenByDescending(record => record.Id)
            .ToListAsync();

        return records
            .Select(record => ToResponse(record, currentUserId, isAdmin))
            .ToList();
    }

    public async Task<RoomConflictRecordResponse?> GetRecordAsync(
        int id,
        int currentUserId,
        bool isAdmin)
    {
        var record = await _context.RoomConflictRecords
            .AsNoTracking()
            .FirstOrDefaultAsync(x => x.Id == id);

        if (record == null)
        {
            return null;
        }

        return ToResponse(record, currentUserId, isAdmin);
    }

    public async Task<RoomConflictRecordResult> CreateManualRecordAsync(
        CreateRoomConflictRecordRequest request,
        int currentUserId,
        bool isAdmin)
    {
        var validationError = await ValidateCreateRequestAsync(request);
        if (validationError != null)
        {
            return RoomConflictRecordResult.BadRequest(validationError);
        }

        var status = request.Status ?? ConflictStatus.Confirmed;

        if (status == ConflictStatus.Detected)
        {
            return RoomConflictRecordResult.BadRequest(
                "Manual conflict reports cannot be created with Detected status.");
        }

        var record = new RoomConflictRecord
        {
            Type = ConflictRecordType.ActualRoomCollision,
            Status = status,
            OccurredAt = request.OccurredAt,
            RoomName = request.RoomName.Trim(),
            ReservationIdA = request.ReservationIdA,
            ReservationIdB = request.ReservationIdB,
            Impact = request.Impact,
            Cause = request.Cause,
            Description = NormalizeText(request.Description),
            Resolution = NormalizeText(request.Resolution),
            DetectionKey = null,
            ReportedByUserId = currentUserId,
            CreatedAt = DateTime.Now
        };

        _context.RoomConflictRecords.Add(record);
        await _context.SaveChangesAsync();

        var response = await GetRecordAsync(record.Id, currentUserId, isAdmin);
        return RoomConflictRecordResult.Success(response!);
    }

    public async Task<RoomConflictRecordResult> UpdateRecordAsync(
        int id,
        UpdateRoomConflictRecordRequest request,
        int currentUserId,
        bool isAdmin)
    {
        var record = await _context.RoomConflictRecords
            .FirstOrDefaultAsync(x => x.Id == id);

        if (record == null)
        {
            return RoomConflictRecordResult.NotFoundResult();
        }

        if (!CanEdit(record, currentUserId, isAdmin))
        {
            return RoomConflictRecordResult.ForbiddenResult();
        }

        var validationError = ValidateUpdateRequest(request);
        if (validationError != null)
        {
            return RoomConflictRecordResult.BadRequest(validationError);
        }

        record.Status = request.Status;
        record.Impact = request.Impact;
        record.Cause = request.Cause;
        record.Description = NormalizeText(request.Description);
        record.Resolution = NormalizeText(request.Resolution);
        record.UpdatedAt = DateTime.Now;

        await _context.SaveChangesAsync();

        var response = await GetRecordAsync(record.Id, currentUserId, isAdmin);
        return RoomConflictRecordResult.Success(response!);
    }

    public async Task<RoomConflictRecordSummaryResponse> GetSummaryAsync(DateTime now)
    {
        var monthStart = new DateTime(now.Year, now.Month, 1);
        var nextMonthStart = monthStart.AddMonths(1);

        return new RoomConflictRecordSummaryResponse
        {
            UnresolvedOverlapsThisMonth = await _context.RoomConflictRecords
                .AsNoTracking()
                .CountAsync(x =>
                    x.Type == ConflictRecordType.UnresolvedReservationOverlap &&
                    x.OccurredAt >= monthStart &&
                    x.OccurredAt < nextMonthStart),

            ConfirmedCollisionsThisMonth = await _context.RoomConflictRecords
                .AsNoTracking()
                .CountAsync(x =>
                    x.Type == ConflictRecordType.ActualRoomCollision &&
                    x.Status == ConflictStatus.Confirmed &&
                    x.OccurredAt >= monthStart &&
                    x.OccurredAt < nextMonthStart),

            HighImpactConflictsThisMonth = await _context.RoomConflictRecords
                .AsNoTracking()
                .CountAsync(x =>
                    x.Impact == ConflictImpact.High &&
                    x.OccurredAt >= monthStart &&
                    x.OccurredAt < nextMonthStart),

            OpenDetectedRecords = await _context.RoomConflictRecords
                .AsNoTracking()
                .CountAsync(x => x.Status == ConflictStatus.Detected)
        };
    }

    private async Task<string?> ValidateCreateRequestAsync(
        CreateRoomConflictRecordRequest request)
    {
        if (request.OccurredAt == default)
        {
            return "OccurredAt is required.";
        }

        if (string.IsNullOrWhiteSpace(request.RoomName))
        {
            return "RoomName is required.";
        }

        if (request.RoomName.Trim().Length > RoomNameMaxLength)
        {
            return $"RoomName must be {RoomNameMaxLength} characters or less.";
        }

        if (!Enum.IsDefined(request.Impact))
        {
            return "Impact is invalid.";
        }

        if (!Enum.IsDefined(request.Cause))
        {
            return "Cause is invalid.";
        }

        if (request.Status.HasValue && !Enum.IsDefined(request.Status.Value))
        {
            return "Status is invalid.";
        }

        var textValidationError = ValidateTextLengths(
            request.Description,
            request.Resolution);

        if (textValidationError != null)
        {
            return textValidationError;
        }

        return await ValidateReservationIdsAsync(
            request.ReservationIdA,
            request.ReservationIdB);
    }

    private static string? ValidateUpdateRequest(
        UpdateRoomConflictRecordRequest request)
    {
        if (!Enum.IsDefined(request.Status))
        {
            return "Status is invalid.";
        }

        if (!Enum.IsDefined(request.Impact))
        {
            return "Impact is invalid.";
        }

        if (!Enum.IsDefined(request.Cause))
        {
            return "Cause is invalid.";
        }

        return ValidateTextLengths(
            request.Description,
            request.Resolution);
    }

    private async Task<string?> ValidateReservationIdsAsync(
        int? reservationIdA,
        int? reservationIdB)
    {
        if (reservationIdA.HasValue &&
            reservationIdB.HasValue &&
            reservationIdA.Value == reservationIdB.Value)
        {
            return "ReservationIdA and ReservationIdB must be different.";
        }

        var reservationIds = new[] { reservationIdA, reservationIdB }
            .Where(id => id.HasValue)
            .Select(id => id!.Value)
            .Distinct()
            .ToList();

        if (reservationIds.Count == 0)
        {
            return null;
        }

        var existingReservationIds = await _context.Reservations
            .AsNoTracking()
            .Where(reservation => reservationIds.Contains(reservation.Id))
            .Select(reservation => reservation.Id)
            .ToListAsync();

        var missingReservationIds = reservationIds
            .Except(existingReservationIds)
            .ToList();

        if (missingReservationIds.Count > 0)
        {
            return $"Reservation was not found: {string.Join(", ", missingReservationIds)}.";
        }

        return null;
    }

    private static string? ValidateTextLengths(
        string? description,
        string? resolution)
    {
        if (!string.IsNullOrWhiteSpace(description) &&
            description.Trim().Length > DescriptionMaxLength)
        {
            return $"Description must be {DescriptionMaxLength} characters or less.";
        }

        if (!string.IsNullOrWhiteSpace(resolution) &&
            resolution.Trim().Length > ResolutionMaxLength)
        {
            return $"Resolution must be {ResolutionMaxLength} characters or less.";
        }

        return null;
    }

    private static string NormalizeText(string? value)
    {
        return value?.Trim() ?? string.Empty;
    }

    private static RoomConflictRecordResponse ToResponse(
        RoomConflictRecord record,
        int currentUserId,
        bool isAdmin)
    {
        return new RoomConflictRecordResponse
        {
            Id = record.Id,
            Type = record.Type,
            Status = record.Status,
            OccurredAt = record.OccurredAt,
            RoomName = record.RoomName,
            ReservationIdA = record.ReservationIdA,
            ReservationIdB = record.ReservationIdB,
            Impact = record.Impact,
            Cause = record.Cause,
            Description = record.Description,
            Resolution = record.Resolution,
            DetectionKey = record.DetectionKey,
            ReportedByUserId = record.ReportedByUserId,
            CreatedAt = record.CreatedAt,
            UpdatedAt = record.UpdatedAt,
            CanEdit = CanEdit(record, currentUserId, isAdmin)
        };
    }

    private static bool CanEdit(
        RoomConflictRecord record,
        int currentUserId,
        bool isAdmin)
    {
        return isAdmin || record.ReportedByUserId == currentUserId;
    }
}
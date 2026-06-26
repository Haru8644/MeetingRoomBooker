using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Reservations;

public static class ReservationRules
{
    public static bool IsRecurring(ReservationModel reservation)
    {
        return ReservationRecurrence.IsRecurring(reservation.RepeatType, reservation.RepeatUntil);
    }

    public static bool IsWeekend(DateTime date)
    {
        return ReservationRecurrence.IsWeekend(date);
    }

    public static void Normalize(ReservationModel reservation)
    {
        reservation.ParticipantIds = (reservation.ParticipantIds ?? new List<int>())
            .Where(id => id > 0)
            .Distinct()
            .ToList();

        if (reservation.UserId > 0 && !reservation.ParticipantIds.Contains(reservation.UserId))
        {
            reservation.ParticipantIds.Insert(0, reservation.UserId);
        }

        reservation.StartTime = reservation.Date.Date + reservation.StartTime.TimeOfDay;
        reservation.EndTime = reservation.Date.Date + reservation.EndTime.TimeOfDay;

        if (string.IsNullOrWhiteSpace(reservation.RepeatType))
        {
            reservation.RepeatType = ReservationRepeatTypes.None;
        }

        reservation.SeriesId = string.IsNullOrWhiteSpace(reservation.SeriesId)
            ? null
            : reservation.SeriesId.Trim();

        if (ReservationRecurrence.IsNone(reservation.RepeatType))
        {
            reservation.RepeatUntil = null;
            reservation.SeriesId = null;
        }
    }

    public static string? Validate(ReservationModel reservation)
    {
        if (string.IsNullOrWhiteSpace(reservation.Room))
        {
            return "会議室を選択してください。";
        }

        if (reservation.StartTime >= reservation.EndTime)
        {
            return "終了時刻は開始時刻より後にしてください。";
        }

        if (reservation.StartTime.Date != reservation.Date.Date || reservation.EndTime.Date != reservation.Date.Date)
        {
            return "予約日時の整合性が取れていません。";
        }

        if (IsRecurring(reservation)
            && reservation.RepeatUntil.HasValue
            && reservation.RepeatUntil.Value.Date < reservation.Date.Date)
        {
            return "繰り返し終了日は予約日以降にしてください。";
        }

        return null;
    }

    public static void SetSeriesIdForCreated(
        ReservationModel reservation,
        string? seriesId = null)
    {
        if (!IsRecurring(reservation))
        {
            reservation.SeriesId = null;
            return;
        }

        reservation.SeriesId = string.IsNullOrWhiteSpace(seriesId)
            ? Guid.NewGuid().ToString("N")
            : seriesId.Trim();
    }

    public static string NormalizeSeriesScope(string? scope)
    {
        return scope switch
        {
            ReservationSeriesScopes.All => ReservationSeriesScopes.All,
            ReservationSeriesScopes.Following => ReservationSeriesScopes.Following,
            _ => ReservationSeriesScopes.Single
        };
    }

    public static List<int> GetNotifiableParticipantIds(ReservationModel reservation)
    {
        return (reservation.ParticipantIds ?? new List<int>())
            .Where(id => id > 0 && id != reservation.UserId)
            .Distinct()
            .ToList();
    }

    public static ReservationModel BuildSeriesRepresentative(
        IReadOnlyCollection<ReservationModel> targets)
    {
        var representative = Clone(
            targets
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .First());

        var stakeholderIds = targets
            .SelectMany(target => (target.ParticipantIds ?? new List<int>()).Append(target.UserId))
            .Where(id => id > 0)
            .Distinct()
            .ToList();

        representative.ParticipantIds = stakeholderIds;
        representative.ParticipantCount = stakeholderIds.Count;

        return representative;
    }

    public static ReservationModel BuildCanceledSeriesRepresentative(
        ReservationModel baseReservation,
        IReadOnlyCollection<ReservationModel> targets)
    {
        var representative = Clone(
            targets
                .OrderBy(x => x.Date)
                .ThenBy(x => x.Id)
                .FirstOrDefault()
            ?? baseReservation);

        var stakeholderIds = targets
            .SelectMany(target => (target.ParticipantIds ?? new List<int>()).Append(target.UserId))
            .Where(id => id > 0)
            .Distinct()
            .ToList();

        representative.ParticipantIds = stakeholderIds;
        representative.ParticipantCount = stakeholderIds.Count;

        return representative;
    }

    public static void ApplyUpdate(
        ReservationModel target,
        ReservationModel source)
    {
        target.Room = source.Room;
        target.NumberOfPeople = source.NumberOfPeople;
        target.Type = source.Type;
        target.Purpose = source.Purpose;
        target.Date = source.Date;
        target.StartTime = source.StartTime;
        target.EndTime = source.EndTime;
        target.ParticipantIds = new List<int>(source.ParticipantIds ?? new List<int>());
        target.Participants = source.Participants;
        target.RepeatType = source.RepeatType;
        target.RepeatUntil = source.RepeatUntil;
        target.SeriesId = source.SeriesId;
    }

    public static ReservationModel Clone(ReservationModel reservation)
    {
        return new ReservationModel
        {
            Id = reservation.Id,
            UserId = reservation.UserId,
            Name = reservation.Name,
            Room = reservation.Room,
            NumberOfPeople = reservation.NumberOfPeople,
            Type = reservation.Type,
            Purpose = reservation.Purpose,
            Date = reservation.Date,
            StartTime = reservation.StartTime,
            EndTime = reservation.EndTime,
            ParticipantIds = new List<int>(reservation.ParticipantIds ?? new List<int>()),
            Participants = reservation.Participants,
            RepeatType = reservation.RepeatType,
            RepeatUntil = reservation.RepeatUntil,
            SeriesId = reservation.SeriesId
        };
    }

    public static ReservationModel CreateRecurringUpdated(
        ReservationModel source,
        ReservationModel updatedTemplate,
        DateTime nextDate)
    {
        return new ReservationModel
        {
            Id = source.Id,
            UserId = source.UserId,
            Name = source.Name,
            Room = updatedTemplate.Room,
            NumberOfPeople = updatedTemplate.NumberOfPeople,
            Type = updatedTemplate.Type,
            Purpose = updatedTemplate.Purpose,
            Date = nextDate,
            StartTime = nextDate.Date + updatedTemplate.StartTime.TimeOfDay,
            EndTime = nextDate.Date + updatedTemplate.EndTime.TimeOfDay,
            ParticipantIds = (updatedTemplate.ParticipantIds ?? new List<int>()).Distinct().ToList(),
            Participants = updatedTemplate.Participants,
            RepeatType = source.RepeatType,
            RepeatUntil = source.RepeatUntil,
            SeriesId = source.SeriesId
        };
    }
}

using MeetingRoomBooker.Api.Services.Reservations;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Tests;

public sealed class ReservationRulesTests
{
    [Fact]
    public void Normalize_inserts_organizer_and_removes_invalid_duplicate_participants()
    {
        var reservation = new ReservationModel
        {
            UserId = 3,
            Date = new DateTime(2026, 6, 1),
            StartTime = new DateTime(2026, 1, 1, 9, 0, 0),
            EndTime = new DateTime(2026, 1, 1, 10, 0, 0),
            ParticipantIds = new List<int> { 0, 2, 2, -1 },
            RepeatType = string.Empty,
            SeriesId = " series-1 "
        };

        ReservationRules.Normalize(reservation);

        Assert.Equal(new List<int> { 3, 2 }, reservation.ParticipantIds);
        Assert.Equal(new DateTime(2026, 6, 1, 9, 0, 0), reservation.StartTime);
        Assert.Equal(new DateTime(2026, 6, 1, 10, 0, 0), reservation.EndTime);
        Assert.Equal("しない", reservation.RepeatType);
        Assert.Null(reservation.RepeatUntil);
        Assert.Null(reservation.SeriesId);
    }

    [Fact]
    public void Normalize_keeps_trimmed_series_id_for_recurring_reservation()
    {
        var reservation = new ReservationModel
        {
            UserId = 1,
            Date = new DateTime(2026, 6, 1),
            StartTime = new DateTime(2026, 6, 1, 9, 0, 0),
            EndTime = new DateTime(2026, 6, 1, 10, 0, 0),
            RepeatType = "毎週",
            RepeatUntil = new DateTime(2026, 6, 30),
            SeriesId = " series-1 "
        };

        ReservationRules.Normalize(reservation);

        Assert.Equal("series-1", reservation.SeriesId);
        Assert.Equal("毎週", reservation.RepeatType);
        Assert.Equal(new DateTime(2026, 6, 30), reservation.RepeatUntil);
    }

    [Fact]
    public void Validate_returns_error_when_room_is_blank()
    {
        var reservation = CreateValidReservation();
        reservation.Room = " ";

        var result = ReservationRules.Validate(reservation);

        Assert.Equal("会議室を選択してください。", result);
    }

    [Fact]
    public void Validate_returns_error_when_end_time_is_not_after_start_time()
    {
        var reservation = CreateValidReservation();
        reservation.EndTime = reservation.StartTime;

        var result = ReservationRules.Validate(reservation);

        Assert.Equal("終了時刻は開始時刻より後にしてください。", result);
    }

    [Fact]
    public void Validate_returns_error_when_recurring_until_is_before_reservation_date()
    {
        var reservation = CreateValidReservation();
        reservation.RepeatType = "毎週";
        reservation.RepeatUntil = reservation.Date.AddDays(-1);

        var result = ReservationRules.Validate(reservation);

        Assert.Equal("繰り返し終了日は予約日以降にしてください。", result);
    }

    [Fact]
    public void SetSeriesIdForCreated_generates_series_id_for_recurring_reservation()
    {
        var reservation = CreateValidReservation();
        reservation.RepeatType = "毎週";
        reservation.RepeatUntil = reservation.Date.AddDays(14);

        ReservationRules.SetSeriesIdForCreated(reservation);

        Assert.False(string.IsNullOrWhiteSpace(reservation.SeriesId));
    }

    [Fact]
    public void SetSeriesIdForCreated_clears_series_id_for_non_recurring_reservation()
    {
        var reservation = CreateValidReservation();
        reservation.RepeatType = "しない";
        reservation.SeriesId = "series-1";

        ReservationRules.SetSeriesIdForCreated(reservation);

        Assert.Null(reservation.SeriesId);
    }

    [Theory]
    [InlineData(ReservationSeriesScopes.Single, ReservationSeriesScopes.Single)]
    [InlineData(ReservationSeriesScopes.Following, ReservationSeriesScopes.Following)]
    [InlineData(ReservationSeriesScopes.All, ReservationSeriesScopes.All)]
    [InlineData("unknown", ReservationSeriesScopes.Single)]
    [InlineData(null, ReservationSeriesScopes.Single)]
    public void NormalizeSeriesScope_returns_supported_scope_or_single(
        string? input,
        string expected)
    {
        var result = ReservationRules.NormalizeSeriesScope(input);

        Assert.Equal(expected, result);
    }

    private static ReservationModel CreateValidReservation()
    {
        return new ReservationModel
        {
            UserId = 1,
            Name = "テストユーザー",
            Room = "大会議室",
            Date = new DateTime(2026, 6, 1),
            StartTime = new DateTime(2026, 6, 1, 9, 0, 0),
            EndTime = new DateTime(2026, 6, 1, 10, 0, 0),
            NumberOfPeople = 2,
            Type = "社内",
            Purpose = "テスト予約",
            ParticipantIds = new List<int> { 1 }
        };
    }
}

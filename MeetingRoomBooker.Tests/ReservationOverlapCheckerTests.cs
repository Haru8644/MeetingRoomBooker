using MeetingRoomBooker.Api.Services.Reservations;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Tests;

public class ReservationOverlapCheckerTests
{
    [Fact]
    public void IsConflicting_returns_true_when_same_room_same_date_and_time_ranges_overlap()
    {
        var existing = CreateReservation(
            room: "会議室A",
            date: new DateTime(2026, 5, 10),
            startHour: 10,
            endHour: 11);

        var requested = CreateReservation(
            room: "会議室A",
            date: new DateTime(2026, 5, 10),
            startHour: 10,
            endHour: 12);

        var result = ReservationOverlapChecker.IsConflicting(existing, requested);

        Assert.True(result);
    }

    [Fact]
    public void IsConflicting_returns_false_when_time_ranges_are_adjacent()
    {
        var existing = CreateReservation(
            room: "会議室A",
            date: new DateTime(2026, 5, 10),
            startHour: 10,
            endHour: 11);

        var requested = CreateReservation(
            room: "会議室A",
            date: new DateTime(2026, 5, 10),
            startHour: 11,
            endHour: 12);

        var result = ReservationOverlapChecker.IsConflicting(existing, requested);

        Assert.False(result);
    }

    [Fact]
    public void IsConflicting_returns_false_when_rooms_are_different()
    {
        var existing = CreateReservation(
            room: "会議室A",
            date: new DateTime(2026, 5, 10),
            startHour: 10,
            endHour: 11);

        var requested = CreateReservation(
            room: "会議室B",
            date: new DateTime(2026, 5, 10),
            startHour: 10,
            endHour: 11);

        var result = ReservationOverlapChecker.IsConflicting(existing, requested);

        Assert.False(result);
    }

    private static ReservationModel CreateReservation(
        string room,
        DateTime date,
        int startHour,
        int endHour)
    {
        return new ReservationModel
        {
            Room = room,
            Date = date.Date,
            StartTime = date.Date.AddHours(startHour),
            EndTime = date.Date.AddHours(endHour)
        };
    }
}
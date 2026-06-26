using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Tests;

public sealed class ReservationRecurrenceTests
{
    [Fact]
    public void BuildTargetDates_WhenRepeatTypeIsNone_ReturnsStartDateOnly()
    {
        var result = ReservationRecurrence.BuildTargetDates(
            new DateTime(2026, 6, 1),
            ReservationRepeatTypes.None,
            new DateTime(2026, 6, 30));

        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 1)
        }, result);
    }

    [Fact]
    public void BuildTargetDates_WhenRepeatTypeIsDaily_ReturnsWeekdayDatesByOneDaySteps()
    {
        var result = ReservationRecurrence.BuildTargetDates(
            new DateTime(2026, 6, 1),
            ReservationRepeatTypes.Daily,
            new DateTime(2026, 6, 8));

        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 1),
            new(2026, 6, 2),
            new(2026, 6, 3),
            new(2026, 6, 4),
            new(2026, 6, 5),
            new(2026, 6, 8)
        }, result);
    }

    [Fact]
    public void BuildTargetDates_WhenRepeatTypeIsWeekly_ReturnsDatesBySevenDaySteps()
    {
        var result = ReservationRecurrence.BuildTargetDates(
            new DateTime(2026, 6, 1),
            ReservationRepeatTypes.Weekly,
            new DateTime(2026, 6, 15));

        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 1),
            new(2026, 6, 8),
            new(2026, 6, 15)
        }, result);
    }

    [Fact]
    public void BuildTargetDates_WhenRepeatTypeIsBiweekly_ReturnsDatesByFourteenDaySteps()
    {
        var result = ReservationRecurrence.BuildTargetDates(
            new DateTime(2026, 6, 1),
            ReservationRepeatTypes.Biweekly,
            new DateTime(2026, 6, 30));

        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 1),
            new(2026, 6, 15),
            new(2026, 6, 29)
        }, result);
    }

    [Fact]
    public void BuildTargetDates_WhenBiweeklyDatesFallOnWeekends_SkipsWeekendDates()
    {
        var result = ReservationRecurrence.BuildTargetDates(
            new DateTime(2026, 6, 6),
            ReservationRepeatTypes.Biweekly,
            new DateTime(2026, 7, 5));

        Assert.Empty(result);
    }

    [Fact]
    public void BuildTargetDates_WhenRepeatUntilFallsBeforeNextBiweeklyDate_DoesNotExceedRepeatUntil()
    {
        var result = ReservationRecurrence.BuildTargetDates(
            new DateTime(2026, 6, 1),
            ReservationRepeatTypes.Biweekly,
            new DateTime(2026, 6, 20));

        Assert.Equal(new List<DateTime>
        {
            new(2026, 6, 1),
            new(2026, 6, 15)
        }, result);
    }
}

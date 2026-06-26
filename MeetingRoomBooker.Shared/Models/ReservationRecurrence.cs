namespace MeetingRoomBooker.Shared.Models;

public static class ReservationRepeatTypes
{
    public const string None = "しない";
    public const string Daily = "毎日";
    public const string Weekly = "毎週";
    public const string Biweekly = "隔週";
}

public static class ReservationRecurrence
{
    public static int GetDayStep(string? repeatType)
    {
        return repeatType switch
        {
            ReservationRepeatTypes.Weekly => 7,
            ReservationRepeatTypes.Biweekly => 14,
            _ => 1
        };
    }

    public static bool IsRecurring(string? repeatType, DateTime? repeatUntil)
    {
        return !IsNone(repeatType) && repeatUntil.HasValue;
    }

    public static bool IsNone(string? repeatType)
    {
        return string.IsNullOrWhiteSpace(repeatType)
            || repeatType == ReservationRepeatTypes.None;
    }

    public static bool IsWeekend(DateTime date)
    {
        return date.DayOfWeek is DayOfWeek.Saturday or DayOfWeek.Sunday;
    }

    public static List<DateTime> BuildTargetDates(
        DateTime startDate,
        string? repeatType,
        DateTime? repeatEndDate)
    {
        if (IsNone(repeatType))
        {
            return new List<DateTime> { startDate.Date };
        }

        var targetDates = new List<DateTime>();
        var currentDate = startDate.Date;
        var endDate = repeatEndDate?.Date ?? startDate.Date;
        var dayStep = GetDayStep(repeatType);

        while (currentDate <= endDate)
        {
            if (!IsWeekend(currentDate))
            {
                targetDates.Add(currentDate);
            }

            currentDate = currentDate.AddDays(dayStep);
        }

        return targetDates;
    }
}

namespace MeetingRoomBooker.Shared.Models;

public static class WorkScheduleRepeatTypes
{
    public const string None = "しない";
    public const string Daily = "毎日";
    public const string Weekly = "毎週";
}

public static class WorkScheduleRecurrence
{
    public static bool IsRecurring(string? repeatType, DateTime? repeatUntil)
    {
        return !IsNone(repeatType) && repeatUntil.HasValue;
    }

    public static bool IsNone(string? repeatType)
    {
        return string.IsNullOrWhiteSpace(repeatType)
            || repeatType == WorkScheduleRepeatTypes.None;
    }

    public static bool IsSupported(string? repeatType)
    {
        return IsNone(repeatType)
            || repeatType == WorkScheduleRepeatTypes.Daily
            || repeatType == WorkScheduleRepeatTypes.Weekly;
    }

    public static bool IsWeekend(DateTime date)
    {
        return date.DayOfWeek is DayOfWeek.Saturday or DayOfWeek.Sunday;
    }

    public static List<DateTime> BuildTargetDates(
        DateTime startDate,
        string? repeatType,
        DateTime? repeatUntil)
    {
        if (IsNone(repeatType))
        {
            return new List<DateTime> { startDate.Date };
        }

        var stepDays = repeatType == WorkScheduleRepeatTypes.Weekly ? 7 : 1;
        var endDate = repeatUntil?.Date ?? startDate.Date;
        var targetDates = new List<DateTime>();

        for (var currentDate = startDate.Date; currentDate <= endDate; currentDate = currentDate.AddDays(stepDays))
        {
            if (!IsWeekend(currentDate))
            {
                targetDates.Add(currentDate);
            }
        }

        return targetDates;
    }
}

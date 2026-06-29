namespace MeetingRoomBooker.Shared.Models;

public static class WorkScheduleSeriesScopes
{
    public const string Single = "single";
    public const string Following = "following";
    public const string All = "all";

    public static bool IsValid(string? scope)
    {
        var normalized = Normalize(scope);

        return normalized is Single or Following or All;
    }

    public static string Normalize(string? scope)
    {
        return string.IsNullOrWhiteSpace(scope)
            ? Single
            : scope.Trim().ToLowerInvariant();
    }
}

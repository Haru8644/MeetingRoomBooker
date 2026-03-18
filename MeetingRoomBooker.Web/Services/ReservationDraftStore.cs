using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Web.Services;

public sealed class ReservationDraftStore
{
    private ReservationDraft? _draft;

    public void Save(ReservationDraft draft)
    {
        _draft = draft with
        {
            Participants = draft.Participants?.Distinct().ToList() ?? new List<int>()
        };
    }

    public ReservationDraft? Get() => _draft;

    public void Clear()
    {
        _draft = null;
    }
}

public sealed record ReservationDraft
{
    public string? Name { get; init; }
    public string? Room { get; init; }
    public string? Type { get; init; }
    public string? Purpose { get; init; }
    public DateTime? Date { get; init; }
    public DateTime? Start { get; init; }
    public DateTime? End { get; init; }
    public string? Repeat { get; init; }
    public DateTime? RepeatUntil { get; init; }
    public List<int> Participants { get; init; } = new();
}

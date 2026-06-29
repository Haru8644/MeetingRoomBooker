using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Models;

public sealed class WorkScheduleEntry
{
	public int Id { get; set; }

	public int CreatedByUserId { get; set; }

	public WorkScheduleEntryType Type { get; set; }

	public string Title { get; set; } = string.Empty;

	public string? SeriesId { get; set; }

	public DateTime Date { get; set; }

	public DateTime? StartTime { get; set; }

	public DateTime? EndTime { get; set; }

	public string? RepeatType { get; set; }

	public DateTime? RepeatUntil { get; set; }

	public LeavePeriod LeavePeriod { get; set; } = LeavePeriod.None;

	public List<int> ParticipantIds { get; set; } = new();

	public string Participants { get; set; } = string.Empty;

	public DateTime CreatedAt { get; set; }

	public DateTime UpdatedAt { get; set; }
}

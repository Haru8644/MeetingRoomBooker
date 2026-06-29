using System;
using System.Collections.Generic;
using System.ComponentModel.DataAnnotations;

namespace MeetingRoomBooker.Shared.Models
{
    public enum WorkScheduleEntryType
    {
        ExternalAppointment = 0,
        WorkFromHome = 1,
        Leave = 2
    }

    public enum LeavePeriod
    {
        None = 0,
        Morning = 1,
        Afternoon = 2,
        FullDay = 3
    }

    public sealed class WorkScheduleEntryModel
    {
        public int Id { get; set; }

        public int CreatedByUserId { get; set; }

        public WorkScheduleEntryType Type { get; set; }

        [StringLength(100, ErrorMessage = "内容は100文字以内で入力してください。")]
        public string Title { get; set; } = string.Empty;

        [StringLength(50)]
        public string? SeriesId { get; set; }

        public DateTime Date { get; set; } = DateTime.Today;

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

    public sealed class CreateWorkScheduleEntryRequest
    {
        public WorkScheduleEntryType Type { get; set; }

        [StringLength(100, ErrorMessage = "内容は100文字以内で入力してください。")]
        public string Title { get; set; } = string.Empty;

        public DateTime Date { get; set; } = DateTime.Today;

        public DateTime? StartTime { get; set; }

        public DateTime? EndTime { get; set; }

        public string? RepeatType { get; set; } = WorkScheduleRepeatTypes.None;

        public DateTime? RepeatUntil { get; set; }

        public LeavePeriod LeavePeriod { get; set; } = LeavePeriod.None;

        public List<int> ParticipantIds { get; set; } = new();

        public string Participants { get; set; } = string.Empty;
    }

    public sealed class UpdateWorkScheduleEntryRequest
    {
        public WorkScheduleEntryType Type { get; set; }

        [StringLength(100, ErrorMessage = "内容は100文字以内で入力してください。")]
        public string Title { get; set; } = string.Empty;

        public DateTime Date { get; set; } = DateTime.Today;

        public DateTime? StartTime { get; set; }

        public DateTime? EndTime { get; set; }

        public string? RepeatType { get; set; } = WorkScheduleRepeatTypes.None;

        public DateTime? RepeatUntil { get; set; }

        public LeavePeriod LeavePeriod { get; set; } = LeavePeriod.None;

        public List<int> ParticipantIds { get; set; } = new();

        public string Participants { get; set; } = string.Empty;
    }
}

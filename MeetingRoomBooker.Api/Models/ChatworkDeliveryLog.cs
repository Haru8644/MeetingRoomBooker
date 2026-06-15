using System;

namespace MeetingRoomBooker.Api.Models
{
    public sealed class ChatworkDeliveryLog
    {
        public int Id { get; set; }
        public int ReservationId { get; set; }
        public int? WorkScheduleEntryId { get; set; }
        public string DeliveryType { get; set; } = string.Empty;
        public string? DeliveryKey { get; set; }
        public int? TargetUserId { get; set; }
        public DateTime ScheduledStartTime { get; set; }
        public string? RoomId { get; set; }
        public string Status { get; set; } = "Succeeded";
        public string? ErrorMessage { get; set; }
        public DateTime AttemptedAt { get; set; } = DateTime.Now;
        public DateTime? SentAt { get; set; } = DateTime.Now;
        public string Message { get; set; } = string.Empty;
        public DateTime CreatedAt { get; set; } = DateTime.Now;
    }
}

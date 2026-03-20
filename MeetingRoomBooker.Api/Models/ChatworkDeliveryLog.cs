using System;

namespace MeetingRoomBooker.Api.Models
{
    public sealed class ChatworkDeliveryLog
    {
        public int Id { get; set; }
        public int ReservationId { get; set; }
        public string DeliveryType { get; set; } = string.Empty;
        public DateTime ScheduledStartTime { get; set; }
        public DateTime SentAt { get; set; } = DateTime.Now;
        public string Message { get; set; } = string.Empty;
        public DateTime CreatedAt { get; set; } = DateTime.Now;
    }
}
using System;

namespace MeetingRoomBooker.Shared.Models
using System;
namespace MeetingRoomBooker.Shared.Models

{
    public class NotificationModel
    {
        public int Id { get; set; }
        public int UserId { get; set; }
        public string Message { get; set; } = string.Empty;
        public int UserId { get; set; }
        public string Message { get; set; } = string.Empty;
        public DateTime CreatedAt { get; set; } = DateTime.Now;
        public DateTime TargetDate { get; set; } = DateTime.Now;
        public bool IsRead { get; set; }
        public string Type { get; set; } = "Info";
        public int? TargetReservationId { get; set; }
        public DateTime CreatedAt { get; set; } = DateTime.Now;
        public DateTime TargetDate { get; set; } = DateTime.Now;
        public bool IsRead { get; set; } = false;
        public string Type { get; set; } = "Info";
        public int? TargetReservationId { get; set; }
    }
}
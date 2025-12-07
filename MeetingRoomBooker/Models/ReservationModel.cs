using System;
using System.ComponentModel.DataAnnotations;

namespace MeetingRoomBooker.Models
{
    public class ReservationModel
    {
        public int Id { get; set; }
        public string Name { get; set; } = "";
        public string Room { get; set; } = "";
        public DateTime Date { get; set; }
        public DateTime StartTime { get; set; }
        public DateTime EndTime { get; set; }
    }
}

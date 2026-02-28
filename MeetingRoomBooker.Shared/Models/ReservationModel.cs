using System;
using System.Collections.Generic;

namespace MeetingRoomBooker.Shared.Models
{
    public class ReservationModel
    {
        public int Id { get; set; }
        public int UserId { get; set; }
        public string Name { get; set; } = string.Empty;
        public string UserName { get => Name; set => Name = value; } 
        public string Room { get; set; } = string.Empty;
        public string RoomName { get => Room; set => Room = value; } 
        public string Type { get; set; } = "社内";
        public bool IsInternal { get => Type == "社内"; set => Type = value ? "社内" : "来客"; }
        public int NumberOfPeople { get; set; } = 1;
        public int ParticipantCount { get => NumberOfPeople; set => NumberOfPeople = value; }
        public string Purpose { get; set; } = string.Empty;
        public DateTime Date { get; set; } = DateTime.Today;
        public DateTime StartTime { get; set; }
        public DateTime EndTime { get; set; }
        public List<int> ParticipantIds { get; set; } = new();
        public string Participants { get; set; } = string.Empty;
        public string RepeatType { get; set; } = "しない";
        public DateTime? RepeatUntil { get; set; }
    }
}
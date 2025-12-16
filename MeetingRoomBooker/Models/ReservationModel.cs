using System;
using System.ComponentModel.DataAnnotations;

namespace MeetingRoomBooker.Models
{
    public class ReservationModel
    {
        public int Id { get; set; }
        [Required(ErrorMessage ="名前は必須です。")]
        [StringLength(20, ErrorMessage ="名前は20文字以内で入力してください。")]
        public string Name { get; set; } = "";
        [Required]
        public string Room { get; set; } = "";
        [Range(1,20,ErrorMessage ="人数は1～20名の間で設定してください。")]
        public int NumberOfPeople { get; set; } = 1;
        public string Type { get; set; } = "社内";
        public string Purpose { get; set; } = "";
        public DateTime Date { get; set; }
        public DateTime StartTime { get; set; }
        public DateTime EndTime { get; set; }
        public int UserId { get; set; }
        public List<int> ParticipantIds { get; set; } = new List<int>();
        public string RepeatType { get; set; } = "しない";
        public DateTime? RepeatUntil { get; set; }
    }
}

using System.ComponentModel.DataAnnotations;

namespace MeetingRoomBooker.Models
{
    public class UserModel
    {
        public int Id { get; set; }
        [Required]
        public string Name { get; set; } = ""; 
        [Required]
        public string Email { get; set; } = ""; 
        [Required]
        public string Password { get; set; } = ""; 
        public string AvatarColor { get; set; } = "#58a6ff"; 
    }
}
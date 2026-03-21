using System.ComponentModel.DataAnnotations;

namespace MeetingRoomBooker.Shared.Models
{
    public class UserModel
    {
        public int Id { get; set; }

        [Required]
        public string Name { get; set; } = string.Empty;

        [Required]
        public string Email { get; set; } = string.Empty;

        [Required]
        public string Password { get; set; } = string.Empty;

        public string AvatarColor { get; set; } = "#58a6ff";
        public bool IsAdmin { get; set; } = false;

        [StringLength(100)]
        public string? ChatworkAccountId { get; set; }
    }
}
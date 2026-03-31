using System.ComponentModel.DataAnnotations;
using System.Text.Json.Serialization;

namespace MeetingRoomBooker.Shared.Models
{
    public class UserModel
    {
        public int Id { get; set; }

        [Required]
        [StringLength(100)]
        public string Name { get; set; } = string.Empty;

        [Required]
        [EmailAddress]
        [StringLength(256)]
        public string Email { get; set; } = string.Empty;

        [JsonIgnore]
        public string Password { get; set; } = string.Empty;

        [JsonIgnore]
        public string? PasswordHash { get; set; }

        public string AvatarColor { get; set; } = "#58a6ff";

        public bool IsAdmin { get; set; }

        [StringLength(100)]
        public string? ChatworkAccountId { get; set; }
    }
}
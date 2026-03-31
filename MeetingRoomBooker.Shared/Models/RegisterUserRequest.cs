using System.ComponentModel.DataAnnotations;

namespace MeetingRoomBooker.Shared.Models
{
    public sealed class RegisterUserRequest
    {
        [Required]
        [StringLength(100)]
        public string Name { get; set; } = string.Empty;

        [Required]
        [EmailAddress]
        [StringLength(256)]
        public string Email { get; set; } = string.Empty;

        [Required]
        public string Password { get; set; } = string.Empty;

        public string AvatarColor { get; set; } = "#58a6ff";

        [StringLength(100)]
        public string? ChatworkAccountId { get; set; }
    }
}
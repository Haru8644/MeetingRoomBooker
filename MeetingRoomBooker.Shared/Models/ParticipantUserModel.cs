namespace MeetingRoomBooker.Shared.Models
{
    public sealed class ParticipantUserModel
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public string AvatarColor { get; set; } = "#58a6ff";
    }
}
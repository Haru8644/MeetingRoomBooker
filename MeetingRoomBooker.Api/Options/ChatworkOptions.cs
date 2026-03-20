namespace MeetingRoomBooker.Api.Options
{
    public sealed class ChatworkOptions
    {
        public bool Enabled { get; set; }
        public string RoomId { get; set; } = string.Empty;
        public string ApiToken { get; set; } = string.Empty;
    }
}

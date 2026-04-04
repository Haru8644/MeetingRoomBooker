namespace MeetingRoomBooker.Shared.Models
{
    public static class ReservationSeriesScopes
    {
        public const string Single = "single";
        public const string Following = "following";
        public const string All = "all";
    }

    public sealed class ReservationSeriesUpdateRequest
    {
        public ReservationModel OriginalReservation { get; set; } = new();
        public ReservationModel UpdatedReservation { get; set; } = new();
        public bool NotifyParticipants { get; set; } = true;
        public string Scope { get; set; } = ReservationSeriesScopes.Single;
    }

    public sealed class ReservationSeriesDeleteRequest
    {
        public ReservationModel OriginalReservation { get; set; } = new();
        public string Scope { get; set; } = ReservationSeriesScopes.Single;
    }
}

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    internal static class ChatworkDeliveryKeys
    {
        public static string Reminder10Minutes(int reservationId, DateTime scheduledStartTime)
        {
            return $"{ChatworkDeliveryTypes.Reminder10Minutes}:reservation:{reservationId}:start:{scheduledStartTime:O}";
        }
    }
}
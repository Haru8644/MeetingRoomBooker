namespace MeetingRoomBooker.Api.Services.Chatwork
{
    internal static class ChatworkDeliveryKeys
    {
        public static string ReservationCreated(int reservationId, int targetUserId)
        {
             return $"{ChatworkDeliveryTypes.ReservationCreated}:reservation:{reservationId}:user:{targetUserId}";
        }

        public static string ReservationUpdated(int reservationId, int targetUserId, string changeId)
        {
            return $"{ChatworkDeliveryTypes.ReservationUpdated}:reservation:{reservationId}:user:{targetUserId}:change:{changeId}";
        }

        public static string Reminder10Minutes(int reservationId, int targetUserId, DateTime scheduledStartTime)
        {
            return $"{ChatworkDeliveryTypes.Reminder10Minutes}:reservation:{reservationId}:user:{targetUserId}:start:{scheduledStartTime:O}";
        }
    }
}
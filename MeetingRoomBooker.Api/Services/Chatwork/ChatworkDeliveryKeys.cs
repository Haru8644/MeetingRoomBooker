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

        public static string ReservationCanceled(int reservationId, int targetUserId)
        {
            return $"{ChatworkDeliveryTypes.ReservationCanceled}:reservation:{reservationId}:user:{targetUserId}";
        }

        public static string Reminder10Minutes(int reservationId, int targetUserId, DateTime scheduledStartTime)
        {
            return $"{ChatworkDeliveryTypes.Reminder10Minutes}:reservation:{reservationId}:user:{targetUserId}:start:{scheduledStartTime:O}";
        }

        public static string WorkScheduleCreated(int workScheduleEntryId, int targetUserId)
        {
            return $"{ChatworkDeliveryTypes.WorkScheduleCreated}:work-schedule:{workScheduleEntryId}:user:{targetUserId}";
        }

        public static string WorkScheduleUpdated(int workScheduleEntryId, int targetUserId, string changeId)
        {
            return $"{ChatworkDeliveryTypes.WorkScheduleUpdated}:work-schedule:{workScheduleEntryId}:user:{targetUserId}:change:{changeId}";
        }

        public static string WorkScheduleDeleted(int workScheduleEntryId, int targetUserId)
        {
            return $"{ChatworkDeliveryTypes.WorkScheduleDeleted}:work-schedule:{workScheduleEntryId}:user:{targetUserId}";
        }

        public static string WorkScheduleReminder10Minutes(int workScheduleEntryId, int targetUserId, DateTime scheduledStartTime)
        {
            return $"{ChatworkDeliveryTypes.WorkScheduleReminder10Minutes}:work-schedule:{workScheduleEntryId}:user:{targetUserId}:start:{scheduledStartTime:O}";
        }
    }
}

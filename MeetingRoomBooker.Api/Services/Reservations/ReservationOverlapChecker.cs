using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Reservations;

public static class ReservationOverlapChecker
{
    public static bool IsConflicting(ReservationModel left, ReservationModel right)
    {
        return left.Room == right.Room
            && left.Date.Date == right.Date.Date
            && left.StartTime < right.EndTime
            && left.EndTime > right.StartTime;
    }
}
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IChatworkRoomResolver
    {
        string ResolveFacilityRoomId(ReservationModel reservation);
        string ResolveStakeholderRoomId();
        string? ResolveReceptionRoomId(ReservationModel reservation);
    }
}
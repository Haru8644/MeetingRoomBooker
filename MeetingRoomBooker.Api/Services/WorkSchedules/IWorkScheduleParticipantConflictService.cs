using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.WorkSchedules;

public interface IWorkScheduleParticipantConflictService
{
    Task<IReadOnlyList<WorkScheduleParticipantConflict>> FindReservationExternalAppointmentConflictsAsync(
        ReservationModel reservation,
        CancellationToken cancellationToken);

    Task<IReadOnlyList<WorkScheduleParticipantConflict>> FindExternalAppointmentConflictsAsync(
        WorkScheduleEntryModel entry,
        CancellationToken cancellationToken);
}

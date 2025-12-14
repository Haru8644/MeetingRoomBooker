using MeetingRoomBooker.Models;

namespace MeetingRoomBooker.Services
{
    public class MockBookingService : IBookingService
    {
        private static List<ReservationModel> _reservations = new List<ReservationModel>
        {
            new ReservationModel {
                Id = 1,
                Name = "Haru",
                Room = "大会議室",
                NumberOfPeople = 5,
                Date = DateTime.Today,
                StartTime = DateTime.Today.AddHours(10),
                EndTime = DateTime.Today.AddHours(11),
                Type = "社内",       
                Purpose = "定例会議" 
            }
        };

        public Task<List<ReservationModel>> GetReservationsAsync() => Task.FromResult(_reservations);
        public Task AddReservationAsync(ReservationModel reservation)
        {
            int newId = _reservations.Any() ? _reservations.Max(r => r.Id) + 1 : 1;
            reservation.Id = newId;
            _reservations.Add(reservation);
            return Task.CompletedTask;
        }

        public Task RemoveReservationAsync(ReservationModel reservation)
        {
            var target = _reservations.FirstOrDefault(r => r.Id == reservation.Id);
            if (target != null) _reservations.Remove(target);
            return Task.CompletedTask;
        }
        public Task UpdateReservationAsync(ReservationModel reservation)
        {
            var target = _reservations.FirstOrDefault(r => r.Id == reservation.Id);
            if (target != null)
            {
                target.Name = reservation.Name;
                target.Room = reservation.Room;
                target.NumberOfPeople = reservation.NumberOfPeople;
                target.Date = reservation.Date;
                target.StartTime = reservation.StartTime;
                target.EndTime = reservation.EndTime;
                target.Type = reservation.Type;
                target.Purpose = reservation.Purpose;
            }
            return Task.CompletedTask;
        }
    }
}
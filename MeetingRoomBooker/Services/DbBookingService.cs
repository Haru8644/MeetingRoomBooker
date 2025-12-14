using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Models;

namespace MeetingRoomBooker.Services
{
    public class DbBookingService : IBookingService
    {
        private readonly AppDbContext _context;

        public DbBookingService(AppDbContext context)
        {
            _context = context;
        }
        public async Task<List<ReservationModel>> GetReservationsAsync()
        {
            await _context.Database.EnsureCreatedAsync();
            return await _context.Reservations.ToListAsync();
        }
        public async Task AddReservationAsync(ReservationModel reservation)
        {
            _context.Reservations.Add(reservation);
            await _context.SaveChangesAsync();
        }
        public async Task RemoveReservationAsync(ReservationModel reservation)
        {
            _context.Reservations.Remove(reservation);
            await _context.SaveChangesAsync();
        }
        public async Task UpdateReservationAsync(ReservationModel reservation)
        {
            var target = await _context.Reservations.FindAsync(reservation.Id);
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
                await _context.SaveChangesAsync();
            }
        }
    }
}
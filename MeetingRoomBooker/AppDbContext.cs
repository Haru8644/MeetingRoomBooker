using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Components.Pages;

namespace MeetingRoomBooker
{
    public class AppDbContext : DbContext
    {
        public AppDbContext(DbContextOptions<AppDbContext> options): base(options)
        { 
            Database.EnsureCreated();
        }
        public DbSet<Reservation.ReservationModel> Reservations { get; set; }
    }
}
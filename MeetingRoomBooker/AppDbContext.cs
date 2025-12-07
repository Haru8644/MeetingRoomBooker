using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Components.Pages;
using MeetingRoomBooker.Models;

namespace MeetingRoomBooker
{
    public class AppDbContext : DbContext
    {
        public AppDbContext(DbContextOptions<AppDbContext> options): base(options)
        { 
            Database.EnsureCreated();
        }
        public DbSet<ReservationModel> Reservations { get; set; }
    }
}
using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Models;

public class AppDbContext : DbContext
{
    public AppDbContext(DbContextOptions<AppDbContext> options): base(options){}
    public DbSet<ReservationModel> Reservations { get; set; } = default!;
}
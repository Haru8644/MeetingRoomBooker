using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker.Models;

public class AppDbContext : DbContext
{
    public AppDbContext(DbContextOptions<AppDbContext> options) : base(options) { }

    public DbSet<ReservationModel> Reservations { get; set; } = default!;

    public DbSet<UserModel> Users { get; set; } = default!;
    public DbSet<NotificationModel> Notifications { get; set; } = default!;

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        base.OnModelCreating(modelBuilder);
        modelBuilder.Entity<ReservationModel>()
            .Property(e => e.ParticipantIds)
            .HasConversion(
                v => string.Join(",", v),          
                v => v.Split(',', StringSplitOptions.RemoveEmptyEntries) 
                      .Select(int.Parse).ToList()
            );
    }
}
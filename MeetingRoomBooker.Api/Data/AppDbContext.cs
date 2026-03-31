using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Data
{
    public class AppDbContext : DbContext
    {
        public AppDbContext(DbContextOptions<AppDbContext> options)
            : base(options)
        {
        }

        public DbSet<UserModel> Users { get; set; }
        public DbSet<ReservationModel> Reservations { get; set; }
        public DbSet<NotificationModel> Notifications { get; set; }
        public DbSet<ChatworkDeliveryLog> ChatworkDeliveryLogs { get; set; }

        protected override void OnModelCreating(ModelBuilder modelBuilder)
        {
            base.OnModelCreating(modelBuilder);

            modelBuilder.Entity<ReservationModel>()
                .Property(e => e.ParticipantIds)
                .HasConversion(
                    v => string.Join(',', v),
                    v => string.IsNullOrWhiteSpace(v)
                        ? new List<int>()
                        : v.Split(',', StringSplitOptions.RemoveEmptyEntries)
                           .Select(int.Parse)
                           .ToList());

            modelBuilder.Entity<UserModel>(entity =>
            {
                entity.Ignore(x => x.Password);
                entity.HasIndex(x => x.Email).IsUnique();
                entity.Property(x => x.Name).HasMaxLength(100);
                entity.Property(x => x.Email).HasMaxLength(256);
                entity.Property(x => x.PasswordHash).HasMaxLength(512);
                entity.Property(x => x.ChatworkAccountId).HasMaxLength(100);
            });

            modelBuilder.Entity<ChatworkDeliveryLog>(entity =>
            {
                entity.HasIndex(x => new { x.ReservationId, x.DeliveryType, x.ScheduledStartTime })
                    .IsUnique();

                entity.Property(x => x.DeliveryType)
                    .HasMaxLength(100);
            });
        }
    }
}
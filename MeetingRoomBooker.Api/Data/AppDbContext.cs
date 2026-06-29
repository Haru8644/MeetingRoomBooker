using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.ChangeTracking;

namespace MeetingRoomBooker.Api.Data
{
    public class AppDbContext : DbContext
    {
        private static readonly ValueComparer<List<int>> IntListComparer = new(
            (left, right) => (left ?? new List<int>()).SequenceEqual(right ?? new List<int>()),
            value => value == null
                ? 0
                : value.Aggregate(0, (hash, item) => HashCode.Combine(hash, item)),
            value => value == null
                ? new List<int>()
                : value.ToList());

        public AppDbContext(DbContextOptions<AppDbContext> options)
            : base(options)
        {
        }

        public DbSet<UserModel> Users { get; set; }
        public DbSet<ReservationModel> Reservations { get; set; }
        public DbSet<NotificationModel> Notifications { get; set; }
        public DbSet<ChatworkDeliveryLog> ChatworkDeliveryLogs { get; set; }
        public DbSet<RoomConflictRecord> RoomConflictRecords { get; set; }
        public DbSet<WorkScheduleEntry> WorkScheduleEntries { get; set; }

        protected override void OnModelCreating(ModelBuilder modelBuilder)
        {
            base.OnModelCreating(modelBuilder);

            modelBuilder.Entity<ReservationModel>(entity =>
            {
                var participantIdsProperty = entity.Property(e => e.ParticipantIds)
                    .HasConversion(
                        value => string.Join(',', value),
                        value => string.IsNullOrWhiteSpace(value)
                            ? new List<int>()
                            : value.Split(',', StringSplitOptions.RemoveEmptyEntries)
                                .Select(int.Parse)
                                .ToList());

                participantIdsProperty.Metadata.SetValueComparer(IntListComparer);

                entity.Property(e => e.SeriesId)
                    .HasMaxLength(50)
                    .IsRequired(false);

                entity.HasIndex(e => e.SeriesId);
            });

            modelBuilder.Entity<UserModel>(entity =>
            {
                entity.HasIndex(x => x.Email).IsUnique();
                entity.Property(x => x.Name).HasMaxLength(100);
                entity.Property(x => x.Email).HasMaxLength(256);
                entity.Property(x => x.PasswordHash).HasMaxLength(512).IsRequired(false);
                entity.Property(x => x.ChatworkAccountId).HasMaxLength(100);
                entity.Property(x => x.ChatworkDirectRoomId).HasMaxLength(100);
                entity.Property(x => x.IsAdmin).HasDefaultValue(false);
            });

            modelBuilder.Entity<NotificationModel>(entity =>
            {
                entity.HasIndex(x => new
                {
                    x.UserId,
                    x.Type,
                    x.TargetReservationId
                });

                entity.HasIndex(x => new
                {
                    x.UserId,
                    x.Type,
                    x.TargetWorkScheduleEntryId
                });
            });

            modelBuilder.Entity<ChatworkDeliveryLog>(entity =>
            {
                entity.HasIndex(x => x.DeliveryKey)
                    .IsUnique();

                entity.HasIndex(x => x.ReservationId);
                entity.HasIndex(x => x.WorkScheduleEntryId);
                entity.HasIndex(x => x.TargetUserId);
                entity.HasIndex(x => x.DeliveryType);

                entity.Property(x => x.DeliveryType)
                    .HasMaxLength(100);

                entity.Property(x => x.DeliveryKey)
                    .HasMaxLength(300)
                    .IsRequired(false);

                entity.Property(x => x.RoomId)
                    .HasMaxLength(100);

                entity.Property(x => x.Status)
                    .HasMaxLength(50);

                entity.Property(x => x.ErrorMessage)
                    .HasMaxLength(1000);
            });

            modelBuilder.Entity<RoomConflictRecord>(entity =>
            {
                entity.HasIndex(x => x.DetectionKey)
                    .IsUnique();

                entity.HasIndex(x => x.OccurredAt);
                entity.HasIndex(x => x.Status);
                entity.HasIndex(x => x.Type);
                entity.HasIndex(x => x.RoomName);

                entity.Property(x => x.RoomName)
                    .HasMaxLength(100);

                entity.Property(x => x.Description)
                    .HasMaxLength(1000);

                entity.Property(x => x.Resolution)
                    .HasMaxLength(1000);

                entity.Property(x => x.DetectionKey)
                    .HasMaxLength(300)
                    .IsRequired(false);
            });

            modelBuilder.Entity<WorkScheduleEntry>(entity =>
            {
                entity.HasIndex(x => x.Date);
                entity.HasIndex(x => x.Type);
                entity.HasIndex(x => x.CreatedByUserId);
                entity.HasIndex(x => x.SeriesId);

                entity.Property(x => x.Title)
                    .HasMaxLength(100);

                entity.Property(x => x.SeriesId)
                    .HasMaxLength(50)
                    .IsRequired(false);

                entity.Property(x => x.Participants)
                    .HasMaxLength(500);

                var participantIdsProperty = entity.Property(x => x.ParticipantIds)
                    .HasConversion(
                        value => string.Join(',', value),
                        value => string.IsNullOrWhiteSpace(value)
                            ? new List<int>()
                            : value.Split(',', StringSplitOptions.RemoveEmptyEntries)
                                .Select(int.Parse)
                                .ToList());

                participantIdsProperty.Metadata.SetValueComparer(IntListComparer);
            });
        }
    }
}

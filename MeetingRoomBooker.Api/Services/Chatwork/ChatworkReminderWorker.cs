using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Options;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ChatworkReminderWorker : BackgroundService
    {
        private readonly IServiceScopeFactory _serviceScopeFactory;
        private readonly ChatworkOptions _options;
        private readonly ILogger<ChatworkReminderWorker> _logger;

        public ChatworkReminderWorker(
            IServiceScopeFactory serviceScopeFactory,
            IOptions<ChatworkOptions> options,
            ILogger<ChatworkReminderWorker> logger)
        {
            _serviceScopeFactory = serviceScopeFactory;
            _options = options.Value;
            _logger = logger;
        }

        protected override async Task ExecuteAsync(CancellationToken stoppingToken)
        {
            while (!stoppingToken.IsCancellationRequested)
            {
                try
                {
                    await ProcessUpcomingRemindersAsync(DateTime.Now, stoppingToken);
                }
                catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
                {
                    break;
                }
                catch (Exception ex)
                {
                    _logger.LogError(ex, "An unexpected error occurred while running the Chatwork reminder worker.");
                }

                try
                {
                    await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
                }
                catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
                {
                    break;
                }
            }
        }

        public async Task ProcessUpcomingRemindersAsync(
            DateTime now,
            CancellationToken cancellationToken)
        {
            if (!_options.Enabled)
            {
                return;
            }

            using var scope = _serviceScopeFactory.CreateScope();
            var context = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var reminderThreshold = now.AddMinutes(10);

            await SendUpcomingReservationRemindersAsync(
                context,
                scope.ServiceProvider.GetRequiredService<IReservationChatworkNotificationService>(),
                now,
                reminderThreshold,
                cancellationToken);

            await SendUpcomingWorkScheduleRemindersAsync(
                context,
                scope.ServiceProvider.GetRequiredService<IWorkScheduleChatworkNotificationService>(),
                now,
                reminderThreshold,
                cancellationToken);
        }

        private async Task SendUpcomingReservationRemindersAsync(
            AppDbContext context,
            IReservationChatworkNotificationService notificationService,
            DateTime now,
            DateTime reminderThreshold,
            CancellationToken cancellationToken)
        {
            var reservations = await context.Reservations
                .AsNoTracking()
                .Where(x => x.StartTime > now && x.StartTime <= reminderThreshold)
                .OrderBy(x => x.StartTime)
                .ToListAsync(cancellationToken);

            if (reservations.Count == 0)
            {
                return;
            }

            foreach (var reservation in reservations)
            {
                try
                {
                    await notificationService.SendReservationReminderAsync(reservation, cancellationToken);
                    _logger.LogInformation(
                        "Processed Chatwork reminder for reservation {ReservationId} scheduled at {StartTime}.",
                        reservation.Id,
                        reservation.StartTime);
                }
                catch (Exception ex)
                {
                    _logger.LogError(
                        ex,
                        "Failed to process Chatwork reminder for reservation {ReservationId}.",
                        reservation.Id);
                }
            }
        }

        private async Task SendUpcomingWorkScheduleRemindersAsync(
            AppDbContext context,
            IWorkScheduleChatworkNotificationService notificationService,
            DateTime now,
            DateTime reminderThreshold,
            CancellationToken cancellationToken)
        {
            var entries = await context.WorkScheduleEntries
                .AsNoTracking()
                .Where(x =>
                    x.Type == WorkScheduleEntryType.ExternalAppointment &&
                    x.StartTime.HasValue &&
                    x.EndTime.HasValue &&
                    x.StartTime > now &&
                    x.StartTime <= reminderThreshold)
                .OrderBy(x => x.StartTime)
                .ToListAsync(cancellationToken);

            if (entries.Count == 0)
            {
                return;
            }

            foreach (var entry in entries)
            {
                try
                {
                    await notificationService.SendReminderAsync(ToModel(entry), cancellationToken);
                    _logger.LogInformation(
                        "Processed Chatwork reminder for work schedule entry {WorkScheduleEntryId} scheduled at {StartTime}.",
                        entry.Id,
                        entry.StartTime);
                }
                catch (Exception ex)
                {
                    _logger.LogError(
                        ex,
                        "Failed to process Chatwork reminder for work schedule entry {WorkScheduleEntryId}.",
                        entry.Id);
                }
            }
        }

        private static WorkScheduleEntryModel ToModel(WorkScheduleEntry entry)
        {
            return new WorkScheduleEntryModel
            {
                Id = entry.Id,
                CreatedByUserId = entry.CreatedByUserId,
                Type = entry.Type,
                Title = entry.Title,
                Date = entry.Date,
                StartTime = entry.StartTime,
                EndTime = entry.EndTime,
                LeavePeriod = entry.LeavePeriod,
                ParticipantIds = new List<int>(entry.ParticipantIds ?? new List<int>()),
                Participants = entry.Participants,
                CreatedAt = entry.CreatedAt,
                UpdatedAt = entry.UpdatedAt
            };
        }
    }
}

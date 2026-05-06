using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Options;
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
                    await SendUpcomingReservationRemindersAsync(stoppingToken);
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

        private async Task SendUpcomingReservationRemindersAsync(CancellationToken cancellationToken)
        {
            if (!_options.Enabled)
            {
                return;
            }

            using var scope = _serviceScopeFactory.CreateScope();
            var context = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var notificationService = scope.ServiceProvider.GetRequiredService<IReservationChatworkNotificationService>();
            var now = DateTime.Now;
            var reminderThreshold = now.AddMinutes(10);

            var reservations = await context.Reservations
                .AsNoTracking()
                .Where(x => x.StartTime > now && x.StartTime <= reminderThreshold)
                .OrderBy(x => x.StartTime)
                .ToListAsync(cancellationToken);

            if (reservations.Count == 0)
            {
                return;
            }

            var reservationIds = reservations
                .Select(x => x.Id)
                .ToList();

            var deliveryLogs = await context.ChatworkDeliveryLogs
                .AsNoTracking()
                .Where(x =>
                    x.DeliveryType == ChatworkDeliveryTypes.Reminder10Minutes &&
                    x.Status == ChatworkDeliveryStatuses.Succeeded &&
                    reservationIds.Contains(x.ReservationId))
                .ToListAsync(cancellationToken);

            var deliveredKeys = deliveryLogs
                .Select(x => x.DeliveryKey ?? ChatworkDeliveryKeys.Reminder10Minutes(x.ReservationId, x.ScheduledStartTime))
                .ToHashSet(StringComparer.Ordinal);

            foreach (var reservation in reservations)
            {
                var deliveryKey = ChatworkDeliveryKeys.Reminder10Minutes(reservation.Id, reservation.StartTime);
                if (deliveredKeys.Contains(deliveryKey))
                {
                    continue;
                }

                try
                {
                    await notificationService.SendReservationReminderAsync(reservation, cancellationToken);

                    var sentAt = DateTime.Now;

                    context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = reservation.Id,
                        DeliveryType = ChatworkDeliveryTypes.Reminder10Minutes,
                        DeliveryKey = deliveryKey,
                        ScheduledStartTime = reservation.StartTime,
                        Status = ChatworkDeliveryStatuses.Succeeded,
                        AttemptedAt = sentAt,
                        SentAt = sentAt,
                        Message = $"Reminder sent for reservation {reservation.Id}.",
                        CreatedAt = sentAt
                    });

                    await context.SaveChangesAsync(cancellationToken);
                    deliveredKeys.Add(deliveryKey);

                    _logger.LogInformation(
                        "Sent Chatwork reminder for reservation {ReservationId} scheduled at {StartTime}.",
                        reservation.Id,
                        reservation.StartTime);
                }
                catch (Exception ex)
                {
                    _logger.LogError(
                        ex,
                        "Failed to send Chatwork reminder for reservation {ReservationId}.",
                        reservation.Id);
                }
            }
        }
    }
}
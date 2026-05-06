using MeetingRoomBooker.Api.Data;
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
    }
}
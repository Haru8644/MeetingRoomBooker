using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Options;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ChatworkReminderWorker : BackgroundService
    {
        private const string ReminderDeliveryType = "Reminder10Minutes";

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
            var chatworkClient = scope.ServiceProvider.GetRequiredService<IChatworkClient>();
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
                    x.DeliveryType == ReminderDeliveryType &&
                    reservationIds.Contains(x.ReservationId))
                .ToListAsync(cancellationToken);

            var deliveredKeys = deliveryLogs
                .Select(x => GetDeliveryKey(x.ReservationId, x.ScheduledStartTime))
                .ToHashSet(StringComparer.Ordinal);

            foreach (var reservation in reservations)
            {
                var deliveryKey = GetDeliveryKey(reservation.Id, reservation.StartTime);
                if (deliveredKeys.Contains(deliveryKey))
                {
                    continue;
                }

                var message = BuildReminderMessage(reservation);

                try
                {
                    await chatworkClient.SendMessageAsync(message, cancellationToken);

                    context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = reservation.Id,
                        DeliveryType = ReminderDeliveryType,
                        ScheduledStartTime = reservation.StartTime,
                        SentAt = DateTime.Now,
                        Message = message,
                        CreatedAt = DateTime.Now
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
        private static string GetDeliveryKey(int reservationId, DateTime scheduledStartTime)
        {
            return $"{reservationId}:{scheduledStartTime:O}";
        }
        private static string BuildReminderMessage(ReservationModel reservation)
        {
            var reservationLabel = GetReservationLabel(reservation);

            return "[info][title]会議室予約リマインド[/title]\n"
                + $"10分後に予約「{reservationLabel}」が開始します。\n"
                + $"利用日: {reservation.Date:yyyy/MM/dd}\n"
                + $"会議室: {reservation.Room}\n"
                + $"時間: {GetTimeRangeText(reservation)}\n"
                + $"予約者: {reservation.Name}\n"
                + "[/info]";
        }
        private static string GetReservationLabel(ReservationModel reservation)
        {
            return string.IsNullOrWhiteSpace(reservation.Purpose)
                ? reservation.Name
                : reservation.Purpose;
        }
        private static string GetTimeRangeText(ReservationModel reservation)
        {
            return $"{reservation.StartTime:HH:mm}〜{reservation.EndTime:HH:mm}";
        }
    }
}
using System.Text;
using MeetingRoomBooker.Shared.Models;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ReservationChatworkNotificationService : IReservationChatworkNotificationService
    {
        private readonly IChatworkClient _chatworkClient;
        private readonly ILogger<ReservationChatworkNotificationService> _logger;

        public ReservationChatworkNotificationService(
            IChatworkClient chatworkClient,
            ILogger<ReservationChatworkNotificationService> logger)
        {
            _chatworkClient = chatworkClient;
            _logger = logger;
        }

        public async Task SendReservationCreatedAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            var message = BuildCreatedMessage(reservation);
            await SafeSendAsync(message, cancellationToken);
        }

        public async Task SendReservationUpdatedAsync(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            CancellationToken cancellationToken = default)
        {
            var message = BuildUpdatedMessage(previousReservation, currentReservation);

            if (string.IsNullOrWhiteSpace(message))
            {
                return;
            }

            await SafeSendAsync(message, cancellationToken);
        }

        public async Task SendReservationCanceledAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            var message = BuildCanceledMessage(reservation);
            await SafeSendAsync(message, cancellationToken);
        }

        private async Task SafeSendAsync(string message, CancellationToken cancellationToken)
        {
            try
            {
                await _chatworkClient.SendMessageAsync(message, cancellationToken);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Failed to send reservation lifecycle notification to Chatwork.");
            }
        }

        private static string BuildCreatedMessage(ReservationModel reservation)
        {
            return "[info][title]会議室予約が作成されました[/title]\n"
                + $"予約名: {GetReservationLabel(reservation)}\n"
                + $"利用日: {reservation.Date:yyyy/MM/dd}\n"
                + $"会議室: {reservation.Room}\n"
                + $"時間: {GetTimeRangeText(reservation)}\n"
                + $"予約者: {reservation.Name}\n"
                + "[/info]";
        }

        private static string BuildUpdatedMessage(ReservationModel previousReservation, ReservationModel currentReservation)
        {
            var changes = new List<string>();

            if (previousReservation.Date.Date != currentReservation.Date.Date)
            {
                changes.Add($"利用日が {previousReservation.Date:yyyy/MM/dd} から {currentReservation.Date:yyyy/MM/dd} に変更されました");
            }

            if (!string.Equals(previousReservation.Room, currentReservation.Room, StringComparison.Ordinal))
            {
                changes.Add($"会議室が {previousReservation.Room} から {currentReservation.Room} に変更されました");
            }

            if (!string.Equals(previousReservation.Type, currentReservation.Type, StringComparison.Ordinal))
            {
                changes.Add($"区分が {previousReservation.Type} から {currentReservation.Type} に変更されました");
            }

            if (previousReservation.StartTime.TimeOfDay != currentReservation.StartTime.TimeOfDay
                || previousReservation.EndTime.TimeOfDay != currentReservation.EndTime.TimeOfDay)
            {
                changes.Add($"時間が {GetTimeRangeText(previousReservation)} から {GetTimeRangeText(currentReservation)} に変更されました");
            }

            var previousLabel = GetReservationLabel(previousReservation);
            var currentLabel = GetReservationLabel(currentReservation);

            if (!string.Equals(previousLabel, currentLabel, StringComparison.Ordinal))
            {
                changes.Add($"予約名が「{previousLabel}」から「{currentLabel}」に変更されました");
            }

            if (changes.Count == 0)
            {
                return string.Empty;
            }

            var builder = new StringBuilder();
            builder.AppendLine("[info][title]会議室予約が変更されました[/title]");
            builder.AppendLine($"予約名: {currentLabel}");

            foreach (var change in changes)
            {
                builder.AppendLine($"- {change}");
            }

            builder.Append("[/info]");
            return builder.ToString();
        }

        private static string BuildCanceledMessage(ReservationModel reservation)
        {
            return "[info][title]会議室予約がキャンセルされました[/title]\n"
                + $"予約名: {GetReservationLabel(reservation)}\n"
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
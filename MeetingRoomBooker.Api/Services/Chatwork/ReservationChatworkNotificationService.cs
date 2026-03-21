using System.Text;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ReservationChatworkNotificationService : IReservationChatworkNotificationService
    {
        private readonly AppDbContext _context;
        private readonly IChatworkClient _chatworkClient;
        private readonly IChatworkRoomResolver _roomResolver;
        private readonly ILogger<ReservationChatworkNotificationService> _logger;

        public ReservationChatworkNotificationService(
            AppDbContext context,
            IChatworkClient chatworkClient,
            IChatworkRoomResolver roomResolver,
            ILogger<ReservationChatworkNotificationService> logger)
        {
            _context = context;
            _chatworkClient = chatworkClient;
            _roomResolver = roomResolver;
            _logger = logger;
        }

        public async Task SendReservationCreatedAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            var usersById = await GetUsersByIdAsync(GetStakeholderUserIds(reservation), cancellationToken);
            var stakeholderUsers = GetUsers(GetStakeholderUserIds(reservation), usersById);

            await SendFacilityNotificationAsync(
                reservation,
                BuildFacilityCreatedMessage(reservation, usersById),
                cancellationToken);

            await SendReceptionNotificationAsync(
                reservation,
                BuildReceptionCreatedMessage(reservation, usersById),
                cancellationToken);

            await SendStakeholderNotificationAsync(
                stakeholderUsers,
                BuildStakeholderCreatedMessage(reservation, usersById, stakeholderUsers),
                cancellationToken);
        }

        public async Task SendReservationUpdatedAsync(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            CancellationToken cancellationToken = default)
        {
            var stakeholderIds = GetStakeholderUserIds(previousReservation)
                .Union(GetStakeholderUserIds(currentReservation))
                .ToList();

            var usersById = await GetUsersByIdAsync(stakeholderIds, cancellationToken);
            var stakeholderUsers = GetUsers(stakeholderIds, usersById);
            var changeLines = BuildChangeLines(previousReservation, currentReservation, usersById);

            if (changeLines.Count == 0)
            {
                return;
            }

            await SendFacilityNotificationAsync(
                currentReservation,
                BuildFacilityUpdatedMessage(currentReservation, usersById, changeLines),
                cancellationToken);

            await SendReceptionNotificationAsync(
                currentReservation,
                BuildReceptionUpdatedMessage(currentReservation, usersById, changeLines),
                cancellationToken);

            await SendStakeholderNotificationAsync(
                stakeholderUsers,
                BuildStakeholderUpdatedMessage(currentReservation, usersById, stakeholderUsers, changeLines),
                cancellationToken);
        }

        public async Task SendReservationCanceledAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            var usersById = await GetUsersByIdAsync(GetStakeholderUserIds(reservation), cancellationToken);
            var stakeholderUsers = GetUsers(GetStakeholderUserIds(reservation), usersById);

            await SendFacilityNotificationAsync(
                reservation,
                BuildFacilityCanceledMessage(reservation, usersById),
                cancellationToken);

            await SendReceptionNotificationAsync(
                reservation,
                BuildReceptionCanceledMessage(reservation, usersById),
                cancellationToken);

            await SendStakeholderNotificationAsync(
                stakeholderUsers,
                BuildStakeholderCanceledMessage(reservation, usersById, stakeholderUsers),
                cancellationToken);
        }

        public async Task SendReservationReminderAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            var usersById = await GetUsersByIdAsync(GetStakeholderUserIds(reservation), cancellationToken);
            var stakeholderUsers = GetUsers(GetStakeholderUserIds(reservation), usersById);

            await SendFacilityNotificationAsync(
                reservation,
                BuildFacilityReminderMessage(reservation, usersById),
                cancellationToken);

            await SendReceptionNotificationAsync(
                reservation,
                BuildReceptionReminderMessage(reservation, usersById),
                cancellationToken);

            await SendStakeholderNotificationAsync(
                stakeholderUsers,
                BuildStakeholderReminderMessage(reservation, usersById, stakeholderUsers),
                cancellationToken);
        }

        private async Task<Dictionary<int, UserModel>> GetUsersByIdAsync(
            IEnumerable<int> userIds,
            CancellationToken cancellationToken)
        {
            var distinctIds = userIds
                .Where(id => id > 0)
                .Distinct()
                .ToList();

            if (distinctIds.Count == 0)
            {
                return new Dictionary<int, UserModel>();
            }

            return await _context.Users
                .Where(user => distinctIds.Contains(user.Id))
                .ToDictionaryAsync(user => user.Id, cancellationToken);
        }

        private async Task SendFacilityNotificationAsync(
            ReservationModel reservation,
            string message,
            CancellationToken cancellationToken)
        {
            try
            {
                var roomId = _roomResolver.ResolveFacilityRoomId(reservation);
                await _chatworkClient.SendMessageAsync(roomId, message, cancellationToken);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Failed to send facility Chatwork notification for reservation {ReservationId}.", reservation.Id);
            }
        }

        private async Task SendReceptionNotificationAsync(
            ReservationModel reservation,
            string message,
            CancellationToken cancellationToken)
        {
            try
            {
                var roomId = _roomResolver.ResolveReceptionRoomId(reservation);
                if (string.IsNullOrWhiteSpace(roomId))
                {
                    return;
                }

                await _chatworkClient.SendMessageAsync(roomId, message, cancellationToken);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Failed to send reception Chatwork notification for reservation {ReservationId}.", reservation.Id);
            }
        }

        private async Task SendStakeholderNotificationAsync(
            IReadOnlyCollection<UserModel> targetUsers,
            string message,
            CancellationToken cancellationToken)
        {
            if (string.IsNullOrWhiteSpace(message))
            {
                return;
            }

            try
            {
                var roomId = _roomResolver.ResolveStakeholderRoomId();
                await _chatworkClient.SendMessageAsync(roomId, AttachMentions(targetUsers, message), cancellationToken);
            }
            catch (Exception ex)
            {
                _logger.LogError(
                    ex,
                    "Failed to send stakeholder Chatwork notification. TargetUserIds: {TargetUserIds}",
                    string.Join(",", targetUsers.Select(user => user.Id)));
            }
        }

        private static List<int> GetStakeholderUserIds(ReservationModel reservation)
        {
            return GetParticipantIds(reservation)
                .Append(reservation.UserId)
                .Where(id => id > 0)
                .Distinct()
                .ToList();
        }

        private static List<int> GetParticipantIds(ReservationModel reservation)
        {
            return (reservation.ParticipantIds ?? new List<int>())
                .Where(id => id > 0 && id != reservation.UserId)
                .Distinct()
                .ToList();
        }

        private static UserModel? GetOrganizerUser(
            IReadOnlyDictionary<int, UserModel> usersById,
            ReservationModel reservation)
        {
            return usersById.TryGetValue(reservation.UserId, out var organizer)
                ? organizer
                : null;
        }

        private static List<UserModel> GetUsers(
            IEnumerable<int> userIds,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return userIds
                .Where(id => id > 0)
                .Distinct()
                .Select(id => usersById.TryGetValue(id, out var user) ? user : null)
                .Where(user => user is not null)
                .Cast<UserModel>()
                .ToList();
        }

        private static string AttachMentions(
            IEnumerable<UserModel> targetUsers,
            string message)
        {
            var mentions = targetUsers
                .Where(user => !string.IsNullOrWhiteSpace(user.ChatworkAccountId))
                .Select(user => $"[To:{user.ChatworkAccountId!.Trim()}]")
                .Distinct()
                .ToList();

            if (mentions.Count == 0)
            {
                return message;
            }

            return $"{string.Concat(mentions)}\n{message}";
        }

        private static string BuildFacilityCreatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return BuildInfoMessage(
                "会議室予約が作成されました",
                new[]
                {
                    $"予約名: {GetReservationLabel(reservation)}",
                    $"利用日: {reservation.Date:yyyy/MM/dd}",
                    $"会議室: {reservation.Room}",
                    $"時間: {GetTimeRangeText(reservation)}",
                    $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                    $"参加者: {GetParticipantNames(reservation, usersById)}"
                });
        }

        private static string BuildFacilityUpdatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<string> changeLines)
        {
            var lines = new List<string>
            {
                $"予約名: {GetReservationLabel(reservation)}",
                $"利用日: {reservation.Date:yyyy/MM/dd}",
                $"会議室: {reservation.Room}",
                $"時間: {GetTimeRangeText(reservation)}",
                $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                $"参加者: {GetParticipantNames(reservation, usersById)}",
                "変更点:"
            };

            lines.AddRange(changeLines.Select(change => $"- {change}"));

            return BuildInfoMessage("会議室予約が変更されました", lines);
        }

        private static string BuildFacilityCanceledMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return BuildInfoMessage(
                "会議室予約がキャンセルされました",
                new[]
                {
                    $"予約名: {GetReservationLabel(reservation)}",
                    $"利用日: {reservation.Date:yyyy/MM/dd}",
                    $"会議室: {reservation.Room}",
                    $"時間: {GetTimeRangeText(reservation)}",
                    $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                    $"参加者: {GetParticipantNames(reservation, usersById)}"
                });
        }

        private static string BuildFacilityReminderMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return BuildInfoMessage(
                "会議室予約リマインド",
                new[]
                {
                    "10分後に開始します。",
                    $"予約名: {GetReservationLabel(reservation)}",
                    $"利用日: {reservation.Date:yyyy/MM/dd}",
                    $"会議室: {reservation.Room}",
                    $"時間: {GetTimeRangeText(reservation)}",
                    $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                    $"参加者: {GetParticipantNames(reservation, usersById)}"
                });
        }

        private static string BuildReceptionCreatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return BuildInfoMessage(
                "受付共有: 来客会議が登録されました",
                new[]
                {
                    $"目的: {GetReservationLabel(reservation)}",
                    $"利用日: {reservation.Date:yyyy/MM/dd}",
                    $"会議室: {reservation.Room}",
                    $"時間: {GetTimeRangeText(reservation)}",
                    $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                    $"参加者: {GetParticipantNames(reservation, usersById)}"
                });
        }

        private static string BuildReceptionUpdatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<string> changeLines)
        {
            var lines = new List<string>
            {
                $"目的: {GetReservationLabel(reservation)}",
                $"利用日: {reservation.Date:yyyy/MM/dd}",
                $"会議室: {reservation.Room}",
                $"時間: {GetTimeRangeText(reservation)}",
                $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                $"参加者: {GetParticipantNames(reservation, usersById)}",
                "変更点:"
            };

            lines.AddRange(changeLines.Select(change => $"- {change}"));

            return BuildInfoMessage("受付共有: 来客会議が変更されました", lines);
        }

        private static string BuildReceptionCanceledMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return BuildInfoMessage(
                "受付共有: 来客会議がキャンセルされました",
                new[]
                {
                    $"目的: {GetReservationLabel(reservation)}",
                    $"利用日: {reservation.Date:yyyy/MM/dd}",
                    $"会議室: {reservation.Room}",
                    $"時間: {GetTimeRangeText(reservation)}",
                    $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                    $"参加者: {GetParticipantNames(reservation, usersById)}"
                });
        }

        private static string BuildReceptionReminderMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            return BuildInfoMessage(
                "受付共有: 来客会議の10分前です",
                new[]
                {
                    $"目的: {GetReservationLabel(reservation)}",
                    $"利用日: {reservation.Date:yyyy/MM/dd}",
                    $"会議室: {reservation.Room}",
                    $"時間: {GetTimeRangeText(reservation)}",
                    $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                    $"参加者: {GetParticipantNames(reservation, usersById)}"
                });
        }

        private static string BuildStakeholderCreatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers)
        {
            return BuildStakeholderSummaryMessage(
                "会議予約を受け付けました",
                "内容を確認してください",
                reservation,
                usersById,
                targetUsers,
                null);
        }

        private static string BuildStakeholderUpdatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers,
            IReadOnlyCollection<string> changeLines)
        {
            return BuildStakeholderSummaryMessage(
                "会議予約が変更されました",
                "変更内容を確認してください",
                reservation,
                usersById,
                targetUsers,
                changeLines.Select(change => $"- {change}").Prepend("変更点:"));
        }

        private static string BuildStakeholderCanceledMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers)
        {
            return BuildStakeholderSummaryMessage(
                "会議予約がキャンセルされました",
                "この予約は無効です",
                reservation,
                usersById,
                targetUsers,
                null);
        }

        private static string BuildStakeholderReminderMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers)
        {
            return BuildStakeholderSummaryMessage(
                "会議開始10分前です",
                "10分後に開始します",
                reservation,
                usersById,
                targetUsers,
                null);
        }

        private static string BuildStakeholderSummaryMessage(
            string title,
            string actionLabel,
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers,
            IEnumerable<string>? extraLines)
        {
            var targetNames = targetUsers
                .Select(user => user.Name)
                .Where(name => !string.IsNullOrWhiteSpace(name))
                .Distinct()
                .ToList();

            var lines = new List<string>
            {
                $"対象: {(targetNames.Count == 0 ? "関係者" : string.Join("、", targetNames))}",
                $"対応: {actionLabel}",
                "[hr]",
                $"目的: {GetReservationLabel(reservation)}",
                $"利用日: {reservation.Date:yyyy/MM/dd}",
                $"会議室: {reservation.Room}",
                $"時間: {GetTimeRangeText(reservation)}",
                $"予約者: {GetOrganizerName(reservation, GetOrganizerUser(usersById, reservation))}",
                $"参加者: {GetParticipantNames(reservation, usersById)}"
            };

            if (extraLines is not null)
            {
                lines.Add("[hr]");
                lines.AddRange(extraLines);
            }

            return BuildInfoMessage(title, lines);
        }

        private static List<string> BuildChangeLines(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            IReadOnlyDictionary<int, UserModel> usersById)
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
                changes.Add($"目的が「{previousLabel}」から「{currentLabel}」に変更されました");
            }

            var previousParticipantIds = GetParticipantIds(previousReservation);
            var currentParticipantIds = GetParticipantIds(currentReservation);

            var addedNames = FormatUserNames(currentParticipantIds.Except(previousParticipantIds), usersById);
            if (!string.IsNullOrWhiteSpace(addedNames))
            {
                changes.Add($"参加メンバーに {addedNames} が追加されました");
            }

            var removedNames = FormatUserNames(previousParticipantIds.Except(currentParticipantIds), usersById);
            if (!string.IsNullOrWhiteSpace(removedNames))
            {
                changes.Add($"参加メンバーから {removedNames} が削除されました");
            }

            return changes;
        }

        private static string BuildInfoMessage(string title, IEnumerable<string> lines)
        {
            var builder = new StringBuilder();
            builder.Append("[info][title]");
            builder.Append(title);
            builder.Append("[/title]");

            foreach (var line in lines.Where(line => !string.IsNullOrWhiteSpace(line)))
            {
                builder.Append('\n');
                builder.Append(line);
            }

            builder.Append("\n[/info]");
            return builder.ToString();
        }

        private static string GetOrganizerName(ReservationModel reservation, UserModel? organizer)
        {
            return organizer?.Name ?? reservation.Name;
        }

        private static string GetParticipantNames(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            var participantNames = GetParticipantIds(reservation)
                .Select(id => usersById.TryGetValue(id, out var user) ? user.Name : $"ユーザー{id}")
                .Where(name => !string.IsNullOrWhiteSpace(name))
                .ToList();

            return participantNames.Count == 0
                ? "なし"
                : string.Join("、", participantNames);
        }

        private static string FormatUserNames(
            IEnumerable<int> userIds,
            IReadOnlyDictionary<int, UserModel> usersById)
        {
            var names = userIds
                .Distinct()
                .Select(id => usersById.TryGetValue(id, out var user) ? user.Name : $"ユーザー{id}")
                .Where(name => !string.IsNullOrWhiteSpace(name))
                .ToList();

            return string.Join("、", names);
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
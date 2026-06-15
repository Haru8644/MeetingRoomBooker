using System.Text;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Models;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ReservationChatworkNotificationService : IReservationChatworkNotificationService
    {
        private readonly AppDbContext _context;
        private readonly IChatworkClient _chatworkClient;
        private readonly IChatworkRoomResolver _roomResolver;
        private readonly IWorkScheduleParticipantConflictService _workScheduleParticipantConflictService;
        private readonly ILogger<ReservationChatworkNotificationService> _logger;

        public ReservationChatworkNotificationService(
            AppDbContext context,
            IChatworkClient chatworkClient,
            IChatworkRoomResolver roomResolver,
            IWorkScheduleParticipantConflictService workScheduleParticipantConflictService,
            ILogger<ReservationChatworkNotificationService> logger)
        {
            _context = context;
            _chatworkClient = chatworkClient;
            _roomResolver = roomResolver;
            _workScheduleParticipantConflictService = workScheduleParticipantConflictService;
            _logger = logger;
        }

        public Task SendReservationCreatedAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            return SendReservationCreatedAsync(
                reservation,
                Array.Empty<ReservationModel>(),
                cancellationToken);
        }

        public async Task SendReservationCreatedAsync(
            ReservationModel reservation,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default)
        {
            var stakeholderIds = GetStakeholderUserIds(reservation);
            var usersById = await GetUsersByIdAsync(stakeholderIds, cancellationToken);
            var stakeholderUsers = GetUsers(stakeholderIds, usersById);
            var participantConflicts = await _workScheduleParticipantConflictService
                .FindReservationExternalAppointmentConflictsAsync(reservation, cancellationToken);

            var stakeholderMessage = BuildStakeholderCreatedMessage(
                reservation,
                usersById,
                stakeholderUsers,
                1,
                overlappingReservations,
                participantConflicts);

            await SendReservationCreatedDirectNotificationsAsync(
                reservation,
                stakeholderUsers,
                stakeholderMessage,
                cancellationToken);
        }

        public Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            CancellationToken cancellationToken = default)
        {
            return SendReservationSeriesCreatedAsync(
                representativeReservation,
                createdCount,
                Array.Empty<ReservationModel>(),
                cancellationToken);
        }

        public async Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default)
        {
            var stakeholderIds = GetStakeholderUserIds(representativeReservation);
            var usersById = await GetUsersByIdAsync(stakeholderIds, cancellationToken);
            var stakeholderUsers = GetUsers(stakeholderIds, usersById);
            var participantConflicts = await _workScheduleParticipantConflictService
                .FindReservationExternalAppointmentConflictsAsync(representativeReservation, cancellationToken);

            var stakeholderMessage = BuildStakeholderCreatedMessage(
                representativeReservation,
                usersById,
                stakeholderUsers,
                createdCount,
                overlappingReservations,
                participantConflicts);

            await SendReservationCreatedDirectNotificationsAsync(
                representativeReservation,
                stakeholderUsers,
                stakeholderMessage,
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
            var participantConflicts = await _workScheduleParticipantConflictService
                .FindReservationExternalAppointmentConflictsAsync(currentReservation, cancellationToken);

            if (changeLines.Count == 0 && participantConflicts.Count == 0)
            {
                return;
            }

            var stakeholderMessage = BuildStakeholderUpdatedMessage(
                currentReservation,
                usersById,
                stakeholderUsers,
                changeLines,
                participantConflicts: participantConflicts);

            var changeId = Guid.NewGuid().ToString("N");

            await SendReservationUpdatedDirectNotificationsAsync(
                currentReservation,
                stakeholderUsers,
                stakeholderMessage,
                changeId,
                cancellationToken);
        }

        public async Task SendReservationSeriesUpdatedAsync(
            ReservationModel representativePreviousReservation,
            ReservationModel representativeCurrentReservation,
            int updatedCount,
            CancellationToken cancellationToken = default)
        {
            var stakeholderIds = GetStakeholderUserIds(representativePreviousReservation)
                .Union(GetStakeholderUserIds(representativeCurrentReservation))
                .ToList();

            var usersById = await GetUsersByIdAsync(stakeholderIds, cancellationToken);
            var stakeholderUsers = GetUsers(stakeholderIds, usersById);
            var changeLines = BuildChangeLines(
                representativePreviousReservation,
                representativeCurrentReservation,
                usersById);

            var participantConflicts = await _workScheduleParticipantConflictService
                .FindReservationExternalAppointmentConflictsAsync(representativeCurrentReservation, cancellationToken);

            if (changeLines.Count == 0 && participantConflicts.Count == 0)
            {
                return;
            }

            var stakeholderMessage = BuildStakeholderUpdatedMessage(
                representativeCurrentReservation,
                usersById,
                stakeholderUsers,
                changeLines,
                updatedCount,
                participantConflicts);

            var changeId = Guid.NewGuid().ToString("N");

            await SendReservationUpdatedDirectNotificationsAsync(
                representativeCurrentReservation,
                stakeholderUsers,
                stakeholderMessage,
                changeId,
                cancellationToken);
        }

        public async Task SendReservationCanceledAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            await SendReservationCanceledDirectNotificationsAsync(
                reservation,
                canceledCount: 1,
                cancellationToken);
        }

        public async Task SendReservationSeriesCanceledAsync(
            ReservationModel representativeReservation,
            int canceledCount,
            CancellationToken cancellationToken = default)
        {
            await SendReservationCanceledDirectNotificationsAsync(
                representativeReservation,
                canceledCount,
                cancellationToken);
        }

        public async Task SendReservationReminderAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            var stakeholderIds = GetStakeholderUserIds(reservation);
            var usersById = await GetUsersByIdAsync(stakeholderIds, cancellationToken);
            var stakeholderUsers = GetUsers(stakeholderIds, usersById);
            var stakeholderMessage = BuildStakeholderReminderMessage(reservation, usersById, stakeholderUsers);

            await SendReservationReminderDirectNotificationsAsync(
                reservation,
                stakeholderUsers,
                stakeholderMessage,
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
                _logger.LogError(
                    ex,
                    "Failed to send facility Chatwork notification for reservation {ReservationId}.",
                    reservation.Id);
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
                _logger.LogError(
                    ex,
                    "Failed to send reception Chatwork notification for reservation {ReservationId}.",
                    reservation.Id);
            }
        }

        private async Task SendStakeholderRoomNotificationAsync(
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

        private async Task SendReservationCreatedDirectNotificationsAsync(
            ReservationModel reservation,
            IReadOnlyCollection<UserModel> targetUsers,
            string message,
            CancellationToken cancellationToken)
        {
            await SendDirectNotificationsAsync(
                reservation,
                targetUsers,
                ChatworkDeliveryTypes.ReservationCreated,
                user => ChatworkDeliveryKeys.ReservationCreated(reservation.Id, user.Id),
                message,
                cancellationToken);
        }

        private async Task SendReservationUpdatedDirectNotificationsAsync(
            ReservationModel reservation,
            IReadOnlyCollection<UserModel> targetUsers,
            string message,
            string changeId,
            CancellationToken cancellationToken)
        {
            await SendDirectNotificationsAsync(
                reservation,
                targetUsers,
                ChatworkDeliveryTypes.ReservationUpdated,
                user => ChatworkDeliveryKeys.ReservationUpdated(reservation.Id, user.Id, changeId),
                message,
                cancellationToken);
        }

        private async Task SendReservationCanceledDirectNotificationsAsync(
            ReservationModel reservation,
            int canceledCount,
            CancellationToken cancellationToken)
        {
            var stakeholderIds = GetStakeholderUserIds(reservation);
            var usersById = await GetUsersByIdAsync(stakeholderIds, cancellationToken);
            var stakeholderUsers = GetUsers(stakeholderIds, usersById);
            var stakeholderMessage = BuildStakeholderCanceledMessage(
                reservation,
                usersById,
                stakeholderUsers,
                canceledCount);

            await SendDirectNotificationsAsync(
                reservation,
                stakeholderUsers,
                ChatworkDeliveryTypes.ReservationCanceled,
                user => ChatworkDeliveryKeys.ReservationCanceled(reservation.Id, user.Id),
                stakeholderMessage,
                cancellationToken);
        }

        private async Task SendReservationReminderDirectNotificationsAsync(
            ReservationModel reservation,
            IReadOnlyCollection<UserModel> targetUsers,
            string message,
            CancellationToken cancellationToken)
        {
            await SendDirectNotificationsAsync(
                reservation,
                targetUsers,
                ChatworkDeliveryTypes.Reminder10Minutes,
                user => ChatworkDeliveryKeys.Reminder10Minutes(reservation.Id, user.Id, reservation.StartTime),
                message,
                cancellationToken);
        }

        private async Task SendDirectNotificationsAsync(
            ReservationModel reservation,
            IReadOnlyCollection<UserModel> targetUsers,
            string deliveryType,
            Func<UserModel, string> buildDeliveryKey,
            string message,
            CancellationToken cancellationToken)
        {
            if (string.IsNullOrWhiteSpace(message) || targetUsers.Count == 0)
            {
                return;
            }

            var uniqueTargetUsers = targetUsers
                .Where(user => user.Id > 0)
                .GroupBy(user => user.Id)
                .Select(group => group.First())
                .ToList();

            var deliveryKeys = uniqueTargetUsers
                .Select(buildDeliveryKey)
                .ToList();

            var existingDeliveryKeyList = await _context.ChatworkDeliveryLogs
                .AsNoTracking()
                .Where(log => log.DeliveryKey != null && deliveryKeys.Contains(log.DeliveryKey))
                .Select(log => log.DeliveryKey!)
                .ToListAsync(cancellationToken);

            var existingDeliveryKeys = existingDeliveryKeyList.ToHashSet(StringComparer.Ordinal);

            foreach (var targetUser in uniqueTargetUsers)
            {
                var deliveryKey = buildDeliveryKey(targetUser);

                if (existingDeliveryKeys.Contains(deliveryKey))
                {
                    continue;
                }

                var attemptedAt = DateTime.Now;
                var roomId = targetUser.ChatworkDirectRoomId?.Trim();

                if (string.IsNullOrWhiteSpace(roomId))
                {
                    _context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = reservation.Id,
                        WorkScheduleEntryId = null,
                        DeliveryType = deliveryType,
                        DeliveryKey = deliveryKey,
                        TargetUserId = targetUser.Id,
                        ScheduledStartTime = reservation.StartTime,
                        RoomId = null,
                        Status = ChatworkDeliveryStatuses.Skipped,
                        ErrorMessage = "ChatworkDirectRoomId is not configured.",
                        AttemptedAt = attemptedAt,
                        SentAt = null,
                        Message = message,
                        CreatedAt = attemptedAt
                    });

                    await _context.SaveChangesAsync(cancellationToken);
                    existingDeliveryKeys.Add(deliveryKey);
                    continue;
                }

                try
                {
                    await _chatworkClient.SendMessageAsync(roomId, message, cancellationToken);

                    var sentAt = DateTime.Now;

                    _context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = reservation.Id,
                        WorkScheduleEntryId = null,
                        DeliveryType = deliveryType,
                        DeliveryKey = deliveryKey,
                        TargetUserId = targetUser.Id,
                        ScheduledStartTime = reservation.StartTime,
                        RoomId = roomId,
                        Status = ChatworkDeliveryStatuses.Succeeded,
                        ErrorMessage = null,
                        AttemptedAt = attemptedAt,
                        SentAt = sentAt,
                        Message = message,
                        CreatedAt = attemptedAt
                    });

                    await _context.SaveChangesAsync(cancellationToken);
                    existingDeliveryKeys.Add(deliveryKey);
                }
                catch (Exception ex)
                {
                    _logger.LogError(
                        ex,
                        "Failed to send direct Chatwork notification. ReservationId: {ReservationId}, DeliveryType: {DeliveryType}, UserId: {UserId}.",
                        reservation.Id,
                        deliveryType,
                        targetUser.Id);

                    _context.ChatworkDeliveryLogs.Add(new ChatworkDeliveryLog
                    {
                        ReservationId = reservation.Id,
                        WorkScheduleEntryId = null,
                        DeliveryType = deliveryType,
                        DeliveryKey = deliveryKey,
                        TargetUserId = targetUser.Id,
                        ScheduledStartTime = reservation.StartTime,
                        RoomId = roomId,
                        Status = ChatworkDeliveryStatuses.Failed,
                        ErrorMessage = ex.Message,
                        AttemptedAt = attemptedAt,
                        SentAt = null,
                        Message = message,
                        CreatedAt = attemptedAt
                    });

                    await _context.SaveChangesAsync(cancellationToken);
                    existingDeliveryKeys.Add(deliveryKey);
                }
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
            IReadOnlyCollection<UserModel> targetUsers,
            int createdCount = 1,
            IReadOnlyCollection<ReservationModel>? overlappingReservations = null,
            IReadOnlyCollection<WorkScheduleParticipantConflict>? participantConflicts = null)
        {
            var extraLines = BuildCreatedExtraLines(
                createdCount,
                overlappingReservations,
                participantConflicts);

            return BuildStakeholderSummaryMessage(
                "会議予約を受け付けました",
                createdCount <= 1
                    ? "内容を確認してください"
                    : "繰り返し予約がまとめて作成されました",
                reservation,
                usersById,
                targetUsers,
                extraLines);
        }

        private static IReadOnlyCollection<string>? BuildCreatedExtraLines(
            int createdCount,
            IReadOnlyCollection<ReservationModel>? overlappingReservations,
            IReadOnlyCollection<WorkScheduleParticipantConflict>? participantConflicts)
        {
            var lines = new List<string>();

            if (createdCount > 1)
            {
                lines.Add($"作成件数: {createdCount}件");
            }

            var overlaps = overlappingReservations?
                .Where(reservation => reservation.Id > 0)
                .GroupBy(reservation => reservation.Id)
                .Select(group => group.First())
                .OrderBy(reservation => reservation.Date)
                .ThenBy(reservation => reservation.StartTime)
                .ToList() ?? new List<ReservationModel>();

            if (overlaps.Count > 0)
            {
                if (lines.Count > 0)
                {
                    lines.Add("[hr]");
                }

                lines.Add("注意: この予約は既存予約と重複しています");
                lines.Add($"重複件数: {overlaps.Count}件");
                lines.Add("重複している既存予約:");

                foreach (var overlap in overlaps.Take(5))
                {
                    lines.Add($"- {overlap.Date:yyyy/MM/dd} {GetTimeRangeText(overlap)} {overlap.Room} / {GetReservationLabel(overlap)}");
                }

                if (overlaps.Count > 5)
                {
                    lines.Add($"- ほか {overlaps.Count - 5}件");
                }
            }

            AddParticipantConflictLines(lines, participantConflicts);

            return lines.Count == 0 ? null : lines;
        }

        private static string BuildStakeholderUpdatedMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers,
            IReadOnlyCollection<string> changeLines,
            int updatedCount = 1,
            IReadOnlyCollection<WorkScheduleParticipantConflict>? participantConflicts = null)
        {
            var extraLines = new List<string>();

            if (changeLines.Count > 0)
            {
                extraLines.Add("変更点:");
                extraLines.AddRange(changeLines.Select(change => $"- {change}"));
            }

            if (updatedCount > 1)
            {
                if (extraLines.Count > 0)
                {
                    extraLines.Add("[hr]");
                }

                extraLines.Add($"更新件数: {updatedCount}件");
            }

            AddParticipantConflictLines(extraLines, participantConflicts);

            return BuildStakeholderSummaryMessage(
                "会議予約が変更されました",
                updatedCount <= 1
                    ? "変更内容を確認してください"
                    : "繰り返し予約がまとめて変更されました",
                reservation,
                usersById,
                targetUsers,
                extraLines);
        }

        private static string BuildStakeholderCanceledMessage(
            ReservationModel reservation,
            IReadOnlyDictionary<int, UserModel> usersById,
            IReadOnlyCollection<UserModel> targetUsers,
            int canceledCount = 1)
        {
            var actionLabel = canceledCount <= 1
                ? "この予約は無効です"
                : "繰り返し予約がまとめて削除されました";

            IEnumerable<string>? extraLines = canceledCount <= 1
                ? null
                : new[] { $"削除件数: {canceledCount}件" };

            return BuildStakeholderSummaryMessage(
                "会議予約がキャンセルされました",
                actionLabel,
                reservation,
                usersById,
                targetUsers,
                extraLines);
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
                var filteredExtraLines = extraLines
                    .Where(line => !string.IsNullOrWhiteSpace(line))
                    .ToList();

                if (filteredExtraLines.Count > 0)
                {
                    lines.Add("[hr]");
                    lines.AddRange(filteredExtraLines);
                }
            }

            return BuildInfoMessage(title, lines);
        }

        private static void AddParticipantConflictLines(
            List<string> lines,
            IReadOnlyCollection<WorkScheduleParticipantConflict>? participantConflicts)
        {
            var conflicts = participantConflicts?
                .GroupBy(conflict => new
                {
                    conflict.ParticipantUserId,
                    conflict.SourceType,
                    conflict.SourceId
                })
                .Select(group => group.First())
                .OrderBy(conflict => conflict.Date)
                .ThenBy(conflict => conflict.StartTime ?? conflict.Date)
                .ThenBy(conflict => conflict.ParticipantName)
                .ToList() ?? new List<WorkScheduleParticipantConflict>();

            if (conflicts.Count == 0)
            {
                return;
            }

            if (lines.Count > 0)
            {
                lines.Add("[hr]");
            }

            lines.Add("注意: 参加者の予定が社外予定と重複しています");
            lines.Add($"重複件数: {conflicts.Count}件");

            foreach (var conflict in conflicts.Take(5))
            {
                lines.Add($"- {conflict.ParticipantName}: {conflict.SourceType}「{conflict.SourceTitle}」 {conflict.Date:yyyy/MM/dd} {GetTimeRangeText(conflict.StartTime, conflict.EndTime)}");
            }

            if (conflicts.Count > 5)
            {
                lines.Add($"- ほか {conflicts.Count - 5}件");
            }
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

        private static string GetTimeRangeText(DateTime? startTime, DateTime? endTime)
        {
            return startTime.HasValue && endTime.HasValue
                ? $"{startTime.Value:HH:mm}〜{endTime.Value:HH:mm}"
                : "終日";
        }
    }
}

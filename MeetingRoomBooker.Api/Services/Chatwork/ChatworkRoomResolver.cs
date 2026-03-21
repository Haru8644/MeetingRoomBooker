using MeetingRoomBooker.Api.Options;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ChatworkRoomResolver : IChatworkRoomResolver
    {
        private readonly ChatworkOptions _options;

        public ChatworkRoomResolver(IOptions<ChatworkOptions> options)
        {
            _options = options.Value;
        }

        public string ResolveFacilityRoomId(ReservationModel reservation)
        {
            if (!string.IsNullOrWhiteSpace(reservation.Room)
                && TryResolveMappedRoomId(reservation.Room, out var mappedRoomId))
            {
                return mappedRoomId;
            }

            if (!string.IsNullOrWhiteSpace(_options.RoomId))
            {
                return _options.RoomId.Trim();
            }

            throw new InvalidOperationException("Chatwork facility room ID is not configured.");
        }

        public string ResolveStakeholderRoomId()
        {
            if (!string.IsNullOrWhiteSpace(_options.StakeholderRoomId))
            {
                return _options.StakeholderRoomId.Trim();
            }

            if (!string.IsNullOrWhiteSpace(_options.RoomId))
            {
                return _options.RoomId.Trim();
            }

            throw new InvalidOperationException("Chatwork stakeholder room ID is not configured.");
        }

        public string? ResolveReceptionRoomId(ReservationModel reservation)
        {
            if (!RequiresReceptionNotification(reservation))
            {
                return null;
            }

            return string.IsNullOrWhiteSpace(_options.ReceptionRoomId)
                ? null
                : _options.ReceptionRoomId.Trim();
        }

        private bool TryResolveMappedRoomId(string roomName, out string roomId)
        {
            foreach (var entry in _options.RoomMappings)
            {
                if (!string.Equals(entry.Key, roomName, StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }

                if (string.IsNullOrWhiteSpace(entry.Value))
                {
                    break;
                }

                roomId = entry.Value.Trim();
                return true;
            }

            roomId = string.Empty;
            return false;
        }

        private static bool RequiresReceptionNotification(ReservationModel reservation)
        {
            return string.Equals(reservation.Type, "来客", StringComparison.Ordinal)
                   || !reservation.IsInternal;
        }
    }
}
using MeetingRoomBooker.Api.Options;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ChatworkClient : IChatworkClient
    {
        private readonly HttpClient _httpClient;
        private readonly ChatworkOptions _options;
        private readonly ILogger<ChatworkClient> _logger;

        public ChatworkClient(
            HttpClient httpClient,
            IOptions<ChatworkOptions> options,
            ILogger<ChatworkClient> logger)
        {
            _httpClient = httpClient;
            _options = options.Value;
            _logger = logger;
        }

        public Task SendMessageAsync(string message, CancellationToken cancellationToken = default)
        {
            if (string.IsNullOrWhiteSpace(_options.RoomId))
            {
                throw new InvalidOperationException("Chatwork default room ID is not configured.");
            }

            return SendMessageAsync(_options.RoomId, message, cancellationToken);
        }

        public async Task SendMessageAsync(string roomId, string message, CancellationToken cancellationToken = default)
        {
            if (!_options.Enabled)
            {
                _logger.LogInformation("Chatwork notification is disabled.");
                return;
            }

            if (string.IsNullOrWhiteSpace(roomId))
            {
                throw new InvalidOperationException("Chatwork room ID is not configured.");
            }

            if (string.IsNullOrWhiteSpace(message))
            {
                throw new ArgumentException("Chatwork message must not be empty.", nameof(message));
            }

            if (string.IsNullOrWhiteSpace(_options.ApiToken))
            {
                throw new InvalidOperationException("Chatwork API token is not configured.");
            }

            using var request = new HttpRequestMessage(
                HttpMethod.Post,
                $"rooms/{roomId.Trim()}/messages");

            request.Headers.Add("X-ChatWorkToken", _options.ApiToken);
            request.Content = new FormUrlEncodedContent(new Dictionary<string, string>
            {
                ["body"] = message
            });

            try
            {
                using var response = await _httpClient.SendAsync(request, cancellationToken);
                var responseBody = await response.Content.ReadAsStringAsync(cancellationToken);

                if (!response.IsSuccessStatusCode)
                {
                    _logger.LogError(
                        "Failed to send Chatwork message. RoomId: {RoomId}, StatusCode: {StatusCode}, Response: {ResponseBody}",
                        roomId,
                        (int)response.StatusCode,
                        responseBody);

                    throw new HttpRequestException(
                        $"Failed to send Chatwork message. StatusCode: {(int)response.StatusCode}, Response: {responseBody}");
                }

                _logger.LogInformation("Chatwork message sent successfully. RoomId: {RoomId}", roomId);
            }
            catch (OperationCanceledException)
            {
                _logger.LogWarning("Chatwork message sending was canceled. RoomId: {RoomId}", roomId);
                throw;
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "An unexpected error occurred while sending a Chatwork message. RoomId: {RoomId}", roomId);
                throw;
            }
        }
    }
}
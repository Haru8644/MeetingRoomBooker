using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using MeetingRoomBooker.Api.Options;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public sealed class ChatworkClient : IChatworkClient
    {
        private readonly HttpClient _httpClient;
        private readonly ChatworkOptions _options;
        private readonly ILogger<ChatworkClient> _logger;

        public ChatworkClient(HttpClient httpClient, IOptions<ChatworkOptions> options, ILogger<ChatworkClient> logger)
        {
            _httpClient = httpClient;
            _options = options.Value;
            _logger = logger;
        }

        public async Task SendMessageAsync(string message, CancellationToken cancellationToken = default)
        {
            if (!_options.Enabled)
            {
                _logger.LogInformation("Chatwork notification is disabled.");
                return;
            }

            if (string.IsNullOrWhiteSpace(message))
            {
                throw new ArgumentException("Chatwork message must not be empty.", nameof(message));
            }

            if (string.IsNullOrWhiteSpace(_options.RoomId))
            {
                throw new InvalidOperationException("Chatwork room ID is not configured.");
            }

            if (string.IsNullOrWhiteSpace(_options.ApiToken))
            {
                throw new InvalidOperationException("Chatwork API token is not configured.");
            }

            using var request = new HttpRequestMessage(HttpMethod.Post, $"rooms/{_options.RoomId}/messages");

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
                        "Failed to send Chatwork message. StatusCode: {StatusCode}, Response: {ResponseBody}",
                        (int)response.StatusCode,
                        responseBody);

                    throw new HttpRequestException($"Failed to send Chatwork message. StatusCode: {(int)response.StatusCode}, Response: {responseBody}");
                }

                _logger.LogInformation("Chatwork message sent successfully.");
            }
            catch (OperationCanceledException)
            {
                _logger.LogWarning("Chatwork message sending was canceled.");
                throw;
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "An unexpected error occurred while sending a Chatwork message.");
                throw;
            }
        }
    }
}
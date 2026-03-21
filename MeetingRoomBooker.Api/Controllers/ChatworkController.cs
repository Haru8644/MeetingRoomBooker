using MeetingRoomBooker.Api.Services.Chatwork;
using Microsoft.AspNetCore.Mvc;

namespace MeetingRoomBooker.Api.Controllers
{
    [ApiController]
    [Route("api/[controller]")]
    public sealed class ChatworkController : ControllerBase
    {
        private readonly IChatworkClient _chatworkClient;
        private readonly ILogger<ChatworkController> _logger;

        public ChatworkController(IChatworkClient chatworkClient, ILogger<ChatworkController> logger)
        {
            _chatworkClient = chatworkClient;
            _logger = logger;
        }

        [HttpPost("test")]
        public async Task<IActionResult> SendTestMessage(
            [FromBody] ChatworkTestRequest request,
            CancellationToken cancellationToken)
        {
            if (request is null)
            {
                return BadRequest("リクエストが空です。");
            }

            if (string.IsNullOrWhiteSpace(request.Message))
            {
                return BadRequest("Message は必須です。");
            }

            try
            {
                if (string.IsNullOrWhiteSpace(request.RoomId))
                {
                    await _chatworkClient.SendMessageAsync(request.Message, cancellationToken);
                }
                else
                {
                    await _chatworkClient.SendMessageAsync(request.RoomId, request.Message, cancellationToken);
                }

                return Ok(new
                {
                    message = "Chatworkへのテスト送信に成功しました。"
                });
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Failed to send Chatwork test message.");
                return StatusCode(500, new
                {
                    message = "Chatworkへのテスト送信に失敗しました。",
                    detail = ex.Message
                });
            }
        }

        public sealed class ChatworkTestRequest
        {
            public string Message { get; set; } = string.Empty;
            public string? RoomId { get; set; }
        }
    }
}
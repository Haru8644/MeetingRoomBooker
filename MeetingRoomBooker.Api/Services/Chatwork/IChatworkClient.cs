namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IChatworkClient
    {
        Task SendMessageAsync(string message, CancellationToken cancellationToken = default);
        Task SendMessageAsync(string roomId, string message, CancellationToken cancellationToken = default);
    }
}

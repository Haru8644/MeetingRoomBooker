namespace MeetingRoomBooker.Api.Services.Chatwork
{
    public interface IChatworkClient
    {
        Task SendMessageAsync(string message, CancellationToken cancellationToken = default);
    }
}

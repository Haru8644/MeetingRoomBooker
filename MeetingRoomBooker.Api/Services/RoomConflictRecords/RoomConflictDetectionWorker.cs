using MeetingRoomBooker.Api.Options;
using Microsoft.Extensions.Options;

namespace MeetingRoomBooker.Api.Services.RoomConflictRecords;

public sealed class RoomConflictDetectionWorker : BackgroundService
{
    private readonly IServiceScopeFactory _serviceScopeFactory;
    private readonly IOptionsMonitor<RoomConflictDetectionOptions> _options;
    private readonly ILogger<RoomConflictDetectionWorker> _logger;

    public RoomConflictDetectionWorker(
        IServiceScopeFactory serviceScopeFactory,
        IOptionsMonitor<RoomConflictDetectionOptions> options,
        ILogger<RoomConflictDetectionWorker> logger)
    {
        _serviceScopeFactory = serviceScopeFactory;
        _options = options;
        _logger = logger;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            var currentOptions = _options.CurrentValue;
            var delay = GetInterval(currentOptions);

            try
            {
                await DetectUnresolvedOverlapsAsync(currentOptions, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                _logger.LogError(
                    ex,
                    "An unexpected error occurred while running the room conflict detection worker.");
            }

            try
            {
                await Task.Delay(delay, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
        }
    }

    private async Task DetectUnresolvedOverlapsAsync(
        RoomConflictDetectionOptions options,
        CancellationToken cancellationToken)
    {
        if (!options.Enabled)
        {
            return;
        }

        using var scope = _serviceScopeFactory.CreateScope();
        var detectionService = scope.ServiceProvider
            .GetRequiredService<IRoomConflictDetectionService>();

        var now = DateTime.Now;
        var lookbackWindow = GetLookbackWindow(options);

        var createdCount = await detectionService.DetectUnresolvedOverlapsAsync(
            now,
            lookbackWindow,
            cancellationToken);

        if (createdCount == 0)
        {
            return;
        }

        _logger.LogInformation(
            "Created {Count} room conflict records for unresolved reservation overlaps.",
            createdCount);
    }

    private static TimeSpan GetInterval(RoomConflictDetectionOptions options)
    {
        if (options.IntervalMinutes <= 0)
        {
            return TimeSpan.FromMinutes(1);
        }

        return TimeSpan.FromMinutes(options.IntervalMinutes);
    }

    private static TimeSpan GetLookbackWindow(RoomConflictDetectionOptions options)
    {
        if (options.LookbackMinutes <= 0)
        {
            return TimeSpan.FromMinutes(15);
        }

        return TimeSpan.FromMinutes(options.LookbackMinutes);
    }
}

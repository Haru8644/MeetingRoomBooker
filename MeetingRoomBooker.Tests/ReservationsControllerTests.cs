using System.Security.Claims;
using MeetingRoomBooker.Api.Controllers;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Services.Chatwork;
using MeetingRoomBooker.Api.Services.Reservations;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Tests;

public sealed class ReservationsControllerTests
{
    [Fact]
    public async Task PostReservation_WhenRoomConflictExistsAndAllowOverlapFalse_ReturnsConflict()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.Reservations.Add(CreateReservation(
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0)));
        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var controller = CreateController(database.Context);
        var request = CreateReservation(
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 30, 0),
            endTime: new DateTime(2026, 6, 1, 11, 30, 0));

        var result = await controller.PostReservation(
            request,
            allowOverlap: false,
            CancellationToken.None);

        Assert.IsType<ConflictObjectResult>(result.Result);
        Assert.Equal(1, await database.Context.Reservations.CountAsync());
    }

    [Fact]
    public async Task PostReservation_WhenRoomConflictExistsAndAllowOverlapTrue_CreatesReservation()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.Reservations.Add(CreateReservation(
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0)));
        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var controller = CreateController(database.Context);
        var request = CreateReservation(
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 30, 0),
            endTime: new DateTime(2026, 6, 1, 11, 30, 0));

        var result = await controller.PostReservation(
            request,
            allowOverlap: true,
            CancellationToken.None);

        var createdResult = Assert.IsType<CreatedAtActionResult>(result.Result);
        var createdReservation = Assert.IsType<ReservationModel>(createdResult.Value);

        Assert.Equal(2, await database.Context.Reservations.CountAsync());
        Assert.Equal(1, createdReservation.UserId);
        Assert.Equal("Test User", createdReservation.Name);
    }

    [Fact]
    public async Task PostReservationSeries_WhenRoomConflictExistsAndAllowOverlapFalse_ReturnsConflict()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.Reservations.Add(CreateReservation(
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0)));
        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var controller = CreateController(database.Context);
        var request = new List<ReservationModel>
        {
            CreateRecurringReservation(
                room: "Room A",
                startTime: new DateTime(2026, 6, 1, 10, 30, 0),
                endTime: new DateTime(2026, 6, 1, 11, 30, 0),
                repeatUntil: new DateTime(2026, 6, 8)),
            CreateRecurringReservation(
                room: "Room A",
                startTime: new DateTime(2026, 6, 8, 10, 30, 0),
                endTime: new DateTime(2026, 6, 8, 11, 30, 0),
                repeatUntil: new DateTime(2026, 6, 8))
        };

        var result = await controller.PostReservationSeries(
            request,
            allowOverlap: false,
            CancellationToken.None);

        Assert.IsType<ConflictObjectResult>(result.Result);
        Assert.Equal(1, await database.Context.Reservations.CountAsync());
    }

    [Fact]
    public async Task PostReservationSeries_WhenRoomConflictExistsAndAllowOverlapTrue_CreatesReservations()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        database.Context.Reservations.Add(CreateReservation(
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0)));
        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var controller = CreateController(database.Context);
        var request = new List<ReservationModel>
        {
            CreateRecurringReservation(
                room: "Room A",
                startTime: new DateTime(2026, 6, 1, 10, 30, 0),
                endTime: new DateTime(2026, 6, 1, 11, 30, 0),
                repeatUntil: new DateTime(2026, 6, 8)),
            CreateRecurringReservation(
                room: "Room A",
                startTime: new DateTime(2026, 6, 8, 10, 30, 0),
                endTime: new DateTime(2026, 6, 8, 11, 30, 0),
                repeatUntil: new DateTime(2026, 6, 8))
        };

        var result = await controller.PostReservationSeries(
            request,
            allowOverlap: true,
            CancellationToken.None);

        var createdResult = Assert.IsType<CreatedAtActionResult>(result.Result);
        var createdReservations = Assert.IsAssignableFrom<IEnumerable<ReservationModel>>(createdResult.Value)
            .ToList();
        var seriesId = Assert.Single(createdReservations.Select(x => x.SeriesId).Distinct());

        Assert.Equal(2, createdReservations.Count);
        Assert.Equal(3, await database.Context.Reservations.CountAsync());
        Assert.False(string.IsNullOrWhiteSpace(seriesId));
    }

    [Fact]
    public async Task PutReservation_WhenOnlyConflictsWithItself_ReturnsNoContent()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var reservation = CreateReservation(
            id: 1,
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0));
        database.Context.Reservations.Add(reservation);
        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var controller = CreateController(database.Context);
        var request = CreateReservation(
            id: reservation.Id,
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0));

        var result = await controller.PutReservation(
            reservation.Id,
            request,
            notifyParticipants: true,
            CancellationToken.None);

        Assert.IsType<NoContentResult>(result);
    }

    [Fact]
    public async Task PutReservation_WhenConflictsWithAnotherReservation_ReturnsConflict()
    {
        using var database = CreateTestDatabase();
        await SeedUsersAsync(database.Context);

        var reservationToUpdate = CreateReservation(
            id: 1,
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 9, 0, 0),
            endTime: new DateTime(2026, 6, 1, 10, 0, 0));
        var conflictingReservation = CreateReservation(
            id: 2,
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 30, 0),
            endTime: new DateTime(2026, 6, 1, 11, 30, 0));

        database.Context.Reservations.AddRange(reservationToUpdate, conflictingReservation);
        await database.Context.SaveChangesAsync();
        database.Context.ChangeTracker.Clear();

        var controller = CreateController(database.Context);
        var request = CreateReservation(
            id: reservationToUpdate.Id,
            room: "Room A",
            startTime: new DateTime(2026, 6, 1, 10, 0, 0),
            endTime: new DateTime(2026, 6, 1, 11, 0, 0));

        var result = await controller.PutReservation(
            reservationToUpdate.Id,
            request,
            notifyParticipants: true,
            CancellationToken.None);

        Assert.IsType<ConflictObjectResult>(result);

        var unchangedReservation = await database.Context.Reservations
            .AsNoTracking()
            .SingleAsync(x => x.Id == reservationToUpdate.Id);

        Assert.Equal(new DateTime(2026, 6, 1, 9, 0, 0), unchangedReservation.StartTime);
        Assert.Equal(new DateTime(2026, 6, 1, 10, 0, 0), unchangedReservation.EndTime);
    }

    private static ReservationsController CreateController(AppDbContext context)
    {
        return new ReservationsController(
            context,
            new ReservationAccessService(context),
            new NoOpReservationChatworkNotificationService(),
            new NoOpReservationNotificationService(),
            new ReservationConflictService(context),
            new ReservationSeriesQueryService(context))
        {
            ControllerContext = new ControllerContext
            {
                HttpContext = new DefaultHttpContext
                {
                    User = CreatePrincipal(userId: 1)
                }
            }
        };
    }

    private static ClaimsPrincipal CreatePrincipal(int userId)
    {
        return new ClaimsPrincipal(new ClaimsIdentity(
            new[] { new Claim(ClaimTypes.NameIdentifier, userId.ToString()) },
            "TestAuth"));
    }

    private static ReservationModel CreateReservation(
        string room,
        DateTime startTime,
        DateTime endTime,
        int id = 0)
    {
        return new ReservationModel
        {
            Id = id,
            UserId = 1,
            Name = "Test User",
            Room = room,
            Date = startTime.Date,
            StartTime = startTime,
            EndTime = endTime,
            NumberOfPeople = 2,
            Type = "Internal",
            Purpose = "Test reservation",
            ParticipantIds = new List<int> { 1 },
            Participants = "Test User"
        };
    }

    private static ReservationModel CreateRecurringReservation(
        string room,
        DateTime startTime,
        DateTime endTime,
        DateTime repeatUntil)
    {
        var reservation = CreateReservation(room, startTime, endTime);
        reservation.RepeatType = "Weekly";
        reservation.RepeatUntil = repeatUntil;

        return reservation;
    }

    private static async Task SeedUsersAsync(AppDbContext context)
    {
        context.Users.Add(new UserModel
        {
            Id = 1,
            Name = "Test User",
            Email = "test@example.com"
        });

        await context.SaveChangesAsync();
    }

    private static TestDatabase CreateTestDatabase()
    {
        var connection = new SqliteConnection("Data Source=:memory:");
        connection.Open();

        var options = new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(connection)
            .Options;

        var context = new AppDbContext(options);
        context.Database.EnsureCreated();

        return new TestDatabase(connection, context);
    }

    private sealed class NoOpReservationNotificationService : IReservationNotificationService
    {
        public Task NotifyParticipantsForCreatedReservationAsync(ReservationModel reservation)
        {
            return Task.CompletedTask;
        }

        public Task NotifyParticipantsForUpdatedReservationAsync(
            ReservationModel previousReservation,
            ReservationModel reservation,
            IReadOnlyCollection<int> previousParticipantIds,
            IReadOnlyCollection<int> currentParticipantIds)
        {
            return Task.CompletedTask;
        }

        public Task NotifyOrganizerForParticipationChangedAsync(
            ReservationModel reservation,
            int actorUserId,
            bool isJoin,
            CancellationToken cancellationToken)
        {
            return Task.CompletedTask;
        }
    }

    private sealed class NoOpReservationChatworkNotificationService : IReservationChatworkNotificationService
    {
        public Task SendReservationCreatedAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationCreatedAsync(
            ReservationModel reservation,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesCreatedAsync(
            ReservationModel representativeReservation,
            int createdCount,
            IReadOnlyCollection<ReservationModel> overlappingReservations,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationUpdatedAsync(
            ReservationModel previousReservation,
            ReservationModel currentReservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesUpdatedAsync(
            ReservationModel representativePreviousReservation,
            ReservationModel representativeCurrentReservation,
            int updatedCount,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationCanceledAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationSeriesCanceledAsync(
            ReservationModel representativeReservation,
            int canceledCount,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }

        public Task SendReservationReminderAsync(
            ReservationModel reservation,
            CancellationToken cancellationToken = default)
        {
            return Task.CompletedTask;
        }
    }

    private sealed class TestDatabase : IDisposable
    {
        private readonly SqliteConnection _connection;

        public TestDatabase(
            SqliteConnection connection,
            AppDbContext context)
        {
            _connection = connection;
            Context = context;
        }

        public AppDbContext Context { get; }

        public void Dispose()
        {
            Context.Dispose();
            _connection.Dispose();
        }
    }
}

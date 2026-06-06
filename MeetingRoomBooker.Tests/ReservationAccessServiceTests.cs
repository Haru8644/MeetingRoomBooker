using System.Security.Claims;
using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Services.Reservations;
using MeetingRoomBooker.Shared.Models;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace MeetingRoomBooker.Tests;

public sealed class ReservationAccessServiceTests
{
    [Fact]
    public async Task GetCurrentUserAsync_returns_user_when_claim_has_existing_user_id()
    {
        using var database = CreateTestDatabase();
        database.Context.Users.Add(new UserModel
        {
            Id = 1,
            Name = "テストユーザー",
            Email = "user@example.com"
        });
        await database.Context.SaveChangesAsync();

        var service = new ReservationAccessService(database.Context);
        var principal = CreatePrincipal(userId: "1");

        var result = await service.GetCurrentUserAsync(principal, CancellationToken.None);

        Assert.NotNull(result);
        Assert.Equal(1, result.Id);
        Assert.Equal("テストユーザー", result.Name);
    }

    [Fact]
    public async Task GetCurrentUserAsync_returns_null_when_user_id_claim_is_missing()
    {
        using var database = CreateTestDatabase();
        var service = new ReservationAccessService(database.Context);
        var principal = CreatePrincipal(userId: null);

        var result = await service.GetCurrentUserAsync(principal, CancellationToken.None);

        Assert.Null(result);
    }

    [Fact]
    public void TryGetCurrentUserId_returns_false_when_user_id_claim_is_not_numeric()
    {
        using var database = CreateTestDatabase();
        var service = new ReservationAccessService(database.Context);
        var principal = CreatePrincipal(userId: "invalid");

        var result = service.TryGetCurrentUserId(principal, out var userId);

        Assert.False(result);
        Assert.Equal(0, userId);
    }

    [Fact]
    public void CanManageReservation_returns_true_for_organizer()
    {
        using var database = CreateTestDatabase();
        var service = new ReservationAccessService(database.Context);
        var principal = CreatePrincipal(userId: "1");
        var reservation = new ReservationModel { UserId = 1 };

        var result = service.CanManageReservation(principal, reservation, currentUserId: 1);

        Assert.True(result);
    }

    [Fact]
    public void CanManageReservation_returns_true_for_admin()
    {
        using var database = CreateTestDatabase();
        var service = new ReservationAccessService(database.Context);
        var principal = CreatePrincipal(userId: "2", isAdmin: true);
        var reservation = new ReservationModel { UserId = 1 };

        var result = service.CanManageReservation(principal, reservation, currentUserId: 2);

        Assert.True(result);
    }

    [Fact]
    public void CanManageReservation_returns_false_for_non_organizer_non_admin()
    {
        using var database = CreateTestDatabase();
        var service = new ReservationAccessService(database.Context);
        var principal = CreatePrincipal(userId: "2");
        var reservation = new ReservationModel { UserId = 1 };

        var result = service.CanManageReservation(principal, reservation, currentUserId: 2);

        Assert.False(result);
    }

    private static ClaimsPrincipal CreatePrincipal(
        string? userId,
        bool isAdmin = false)
    {
        var claims = new List<Claim>();

        if (!string.IsNullOrWhiteSpace(userId))
        {
            claims.Add(new Claim(ClaimTypes.NameIdentifier, userId));
        }

        if (isAdmin)
        {
            claims.Add(new Claim(ClaimTypes.Role, "Admin"));
        }

        return new ClaimsPrincipal(new ClaimsIdentity(claims, "TestAuth"));
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

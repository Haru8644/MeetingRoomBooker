using MeetingRoomBooker.Api.Data;
using MeetingRoomBooker.Api.Options;
using MeetingRoomBooker.Api.Services.Chatwork;
using Microsoft.EntityFrameworkCore;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddControllers();
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen();

var connectionString = builder.Configuration.GetConnectionString("DefaultConnection");
builder.Services.AddDbContext<AppDbContext>(options =>
    options.UseSqlite(connectionString));

builder.Services.Configure<ChatworkOptions>(
    builder.Configuration.GetSection("Chatwork"));

builder.Services.AddHttpClient<IChatworkClient, ChatworkClient>(client =>
{
    client.BaseAddress = new Uri("https://api.chatwork.com/v2/");
});

builder.Services.AddScoped<IReservationChatworkNotificationService, ReservationChatworkNotificationService>();
builder.Services.AddHostedService<ChatworkReminderWorker>();

builder.Services.AddCors(options =>
{
    options.AddPolicy("AllowAll", policy =>
    {
        policy.AllowAnyOrigin()
              .AllowAnyMethod()
              .AllowAnyHeader();
    });
});

var app = builder.Build();

using (var scope = app.Services.CreateScope())
{
    var services = scope.ServiceProvider;

    try
    {
        var context = services.GetRequiredService<AppDbContext>();
        context.Database.Migrate();
    }
    catch (Exception ex)
    {
        var logger = services.GetRequiredService<ILogger<Program>>();
        logger.LogError(ex, "Database initialization failed.");
    }
}

if (app.Environment.IsDevelopment())
{
    app.UseSwagger();
    app.UseSwaggerUI();
}

app.UseHttpsRedirection();
app.UseCors("AllowAll");
app.UseAuthorization();
app.MapControllers();
app.Run();
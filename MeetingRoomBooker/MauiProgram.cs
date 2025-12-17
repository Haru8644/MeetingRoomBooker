using MeetingRoomBooker.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using Microsoft.FluentUI.AspNetCore.Components;
using MeetingRoomBooker.Models;
using System.Reflection;

namespace MeetingRoomBooker
{
    public static class MauiProgram
    {
        public static MauiApp CreateMauiApp()
        {
            var builder = MauiApp.CreateBuilder();
            builder.UseMauiApp<App>().ConfigureFonts(fonts =>
            {
                fonts.AddFont("OpenSans-Regular.ttf", "OpenSansRegular");
            });

            builder.Services.AddMauiBlazorWebView();

#if DEBUG
            builder.Services.AddBlazorWebViewDeveloperTools();
            builder.Logging.AddDebug();
#endif
            var assembly = Assembly.GetExecutingAssembly();
            using var stream = assembly.GetManifestResourceStream("MeetingRoomBooker.appsettings.json");
            var config = new ConfigurationBuilder()
                .AddJsonStream(stream)
                .Build();
            builder.Configuration.AddConfiguration(config);
            var connectionString = builder.Configuration.GetConnectionString("DefaultConnection");
            builder.Services.AddDbContextFactory<AppDbContext>(options =>
                options.UseSqlServer(connectionString));
            builder.Services.AddSingleton<IBookingService, DbBookingService>();
            builder.Services.AddFluentUIComponents();
            return builder.Build();
        }
    }
}
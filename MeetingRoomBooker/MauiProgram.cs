using MeetingRoomBooker.Services;
using Microsoft.Extensions.Logging;
using Microsoft.FluentUI.AspNetCore.Components;

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
            var apiUrl = "http://localhost:5123/";
            builder.Services.AddScoped(sp => new HttpClient { BaseAddress = new Uri(apiUrl) });
            builder.Services.AddScoped<IBookingService, ApiBookingService>();
            builder.Services.AddFluentUIComponents();
            return builder.Build();
        }
    }
}
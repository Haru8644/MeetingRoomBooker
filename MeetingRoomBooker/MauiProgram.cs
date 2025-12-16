using MeetingRoomBooker;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using MeetingRoomBooker.Services;
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
            builder.Services.AddSingleton<IBookingService, MockBookingService>();
            builder.Services.AddFluentUIComponents();
            var app = builder.Build();
            return app;
        }
    }
}
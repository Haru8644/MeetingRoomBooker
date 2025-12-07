using Microsoft.Extensions.Logging;
using Microsoft.EntityFrameworkCore;
using MeetingRoomBooker;

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
            string dbPath = Path.Combine(FileSystem.AppDataDirectory, "app.db");
            builder.Services.AddDbContext<AppDbContext>(options =>options.UseSqlite($"Data Source={dbPath}"));
            return builder.Build();
        }
    }
}

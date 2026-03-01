using MeetingRoomBooker.Web;
using MeetingRoomBooker.Web.Services;
using Microsoft.AspNetCore.Components.Web;
using Microsoft.AspNetCore.Components.WebAssembly.Hosting;
using Microsoft.FluentUI.AspNetCore.Components;

var builder = WebAssemblyHostBuilder.CreateDefault(args);

builder.RootComponents.Add<App>("#app");
builder.RootComponents.Add<HeadOutlet>("head::after");

builder.Services.AddScoped(_ => new HttpClient
{
    BaseAddress = new Uri(builder.HostEnvironment.BaseAddress)
});

builder.Services.AddFluentUIComponents();
builder.Services.AddScoped<ThemeService>();

builder.Services.AddScoped<
    MeetingRoomBooker.Shared.Services.IBookingService,
    MeetingRoomBooker.Web.Services.MockBookingService>();
await builder.Build().RunAsync();
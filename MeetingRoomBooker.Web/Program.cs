using Microsoft.AspNetCore.Components.Web;
using Microsoft.AspNetCore.Components.WebAssembly.Hosting;
using MeetingRoomBooker.Web;
using Microsoft.FluentUI.AspNetCore.Components;
using MeetingRoomBooker.Services;  
using MeetingRoomBooker.Web.Services;  

var builder = WebAssemblyHostBuilder.CreateDefault(args);
builder.RootComponents.Add<App>("#app");
builder.RootComponents.Add<HeadOutlet>("head::after");
var apiUrl = "https://localhost:7005/";
builder.Services.AddScoped(sp => new HttpClient { BaseAddress = new Uri(apiUrl) });
builder.Services.AddHttpClient<IBookingService, ApiBookingService>(client =>
{
    client.BaseAddress = new Uri(apiUrl);
});
builder.Services.AddFluentUIComponents();

await builder.Build().RunAsync();
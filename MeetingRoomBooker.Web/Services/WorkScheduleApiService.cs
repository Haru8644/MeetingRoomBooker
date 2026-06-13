using System.Globalization;
using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Components.WebAssembly.Http;

namespace MeetingRoomBooker.Web.Services;

public sealed class WorkScheduleApiService
{
    private readonly HttpClient _httpClient;

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true
    };

    public WorkScheduleApiService(HttpClient httpClient)
    {
        _httpClient = httpClient;
    }

    public async Task<List<WorkScheduleEntryModel>> GetEntriesAsync(
        DateTime? from,
        DateTime? to,
        CancellationToken cancellationToken = default)
    {
        var query = BuildDateRangeQuery(from, to);
        var response = await SendAsync(
            HttpMethod.Get,
            $"api/work-schedule-entries{query}",
            cancellationToken: cancellationToken);

        await EnsureSuccessAsync(response, "勤務予定の取得に失敗しました。");

        return await ReadFromJsonAsync<List<WorkScheduleEntryModel>>(response, cancellationToken)
            ?? new List<WorkScheduleEntryModel>();
    }

    public async Task<WorkScheduleEntryModel?> GetEntryAsync(
        int id,
        CancellationToken cancellationToken = default)
    {
        var response = await SendAsync(
            HttpMethod.Get,
            $"api/work-schedule-entries/{id}",
            cancellationToken: cancellationToken);

        if (response.StatusCode == HttpStatusCode.NotFound)
        {
            return null;
        }

        await EnsureSuccessAsync(response, $"勤務予定 id={id} の取得に失敗しました。");

        return await ReadFromJsonAsync<WorkScheduleEntryModel>(response, cancellationToken);
    }

    public async Task<WorkScheduleEntryModel> CreateEntryAsync(
        CreateWorkScheduleEntryRequest request,
        CancellationToken cancellationToken = default)
    {
        var response = await SendAsync(
            HttpMethod.Post,
            "api/work-schedule-entries",
            request,
            cancellationToken);

        await EnsureSuccessAsync(response, "勤務予定の登録に失敗しました。");

        return await ReadFromJsonAsync<WorkScheduleEntryModel>(response, cancellationToken)
            ?? throw new InvalidOperationException("勤務予定の登録結果が空です。");
    }

    public async Task<WorkScheduleEntryModel> UpdateEntryAsync(
        int id,
        UpdateWorkScheduleEntryRequest request,
        CancellationToken cancellationToken = default)
    {
        var response = await SendAsync(
            HttpMethod.Put,
            $"api/work-schedule-entries/{id}",
            request,
            cancellationToken);

        await EnsureSuccessAsync(response, $"勤務予定 id={id} の更新に失敗しました。");

        return await ReadFromJsonAsync<WorkScheduleEntryModel>(response, cancellationToken)
            ?? throw new InvalidOperationException("勤務予定の更新結果が空です。");
    }

    public async Task DeleteEntryAsync(
        int id,
        CancellationToken cancellationToken = default)
    {
        var response = await SendAsync(
            HttpMethod.Delete,
            $"api/work-schedule-entries/{id}",
            cancellationToken: cancellationToken);

        await EnsureSuccessAsync(response, $"勤務予定 id={id} の削除に失敗しました。");
    }

    private async Task<HttpResponseMessage> SendAsync(
        HttpMethod method,
        string requestUri,
        object? body = null,
        CancellationToken cancellationToken = default)
    {
        var request = new HttpRequestMessage(method, requestUri);
        request.SetBrowserRequestCredentials(BrowserRequestCredentials.Include);

        if (body != null)
        {
            request.Content = JsonContent.Create(body);
        }

        return await _httpClient.SendAsync(request, cancellationToken);
    }

    private static async Task<T?> ReadFromJsonAsync<T>(
        HttpResponseMessage response,
        CancellationToken cancellationToken)
    {
        if (response.Content.Headers.ContentLength == 0)
        {
            return default;
        }

        return await response.Content.ReadFromJsonAsync<T>(
            JsonOptions,
            cancellationToken);
    }

    private static async Task EnsureSuccessAsync(
        HttpResponseMessage response,
        string fallbackMessage)
    {
        if (response.IsSuccessStatusCode)
        {
            return;
        }

        var details = await response.Content.ReadAsStringAsync();

        throw new InvalidOperationException(
            string.IsNullOrWhiteSpace(details)
                ? fallbackMessage
                : details);
    }

    private static string BuildDateRangeQuery(DateTime? from, DateTime? to)
    {
        var parameters = new List<string>();

        if (from.HasValue)
        {
            parameters.Add($"from={FormatDate(from.Value)}");
        }

        if (to.HasValue)
        {
            parameters.Add($"to={FormatDate(to.Value)}");
        }

        return parameters.Count == 0
            ? string.Empty
            : $"?{string.Join("&", parameters)}";
    }

    private static string FormatDate(DateTime value)
    {
        return WebUtility.UrlEncode(
            value.Date.ToString("yyyy-MM-dd", CultureInfo.InvariantCulture));
    }
}

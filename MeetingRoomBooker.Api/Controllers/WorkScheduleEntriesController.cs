using System.Security.Claims;
using MeetingRoomBooker.Api.Services.WorkSchedules;
using MeetingRoomBooker.Shared.Models;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

namespace MeetingRoomBooker.Api.Controllers;

[ApiController]
[Authorize]
[Route("api/work-schedule-entries")]
public sealed class WorkScheduleEntriesController : ControllerBase
{
    private readonly IWorkScheduleEntryService _service;

    public WorkScheduleEntriesController(IWorkScheduleEntryService service)
    {
        _service = service;
    }

    [HttpGet]
    public async Task<ActionResult<IReadOnlyList<WorkScheduleEntryModel>>> GetEntries(
        [FromQuery] DateTime? from,
        [FromQuery] DateTime? to,
        CancellationToken cancellationToken)
    {
        if (from.HasValue && to.HasValue && from.Value.Date > to.Value.Date)
        {
            return BadRequest("取得開始日は取得終了日以前にしてください。");
        }

        var entries = await _service.GetEntriesAsync(
            from,
            to,
            cancellationToken);

        return Ok(entries);
    }

    [HttpGet("{id:int}")]
    public async Task<ActionResult<WorkScheduleEntryModel>> GetEntry(
        int id,
        CancellationToken cancellationToken)
    {
        var entry = await _service.GetEntryAsync(
            id,
            cancellationToken);

        if (entry == null)
        {
            return NotFound();
        }

        return Ok(entry);
    }

    [HttpPost]
    public async Task<ActionResult<WorkScheduleEntryModel>> CreateEntry(
        [FromBody] CreateWorkScheduleEntryRequest request,
        CancellationToken cancellationToken)
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var result = await _service.CreateEntryAsync(
            request,
            currentUserId,
            IsCurrentUserAdmin(),
            cancellationToken);

        if (result.ErrorMessage != null)
        {
            return BadRequest(result.ErrorMessage);
        }

        return CreatedAtAction(
            nameof(GetEntry),
            new { id = result.Entry!.Id },
            result.Entry);
    }

    [HttpPut("{id:int}")]
    public async Task<ActionResult<WorkScheduleEntryModel>> UpdateEntry(
        int id,
        [FromBody] UpdateWorkScheduleEntryRequest request,
        CancellationToken cancellationToken)
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var result = await _service.UpdateEntryAsync(
            id,
            request,
            currentUserId,
            IsCurrentUserAdmin(),
            cancellationToken);

        if (result.NotFound)
        {
            return NotFound();
        }

        if (result.Forbidden)
        {
            return Forbid();
        }

        if (result.ErrorMessage != null)
        {
            return BadRequest(result.ErrorMessage);
        }

        return Ok(result.Entry);
    }

    [HttpDelete("{id:int}")]
    public async Task<IActionResult> DeleteEntry(
        int id,
        CancellationToken cancellationToken)
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var result = await _service.DeleteEntryAsync(
            id,
            currentUserId,
            IsCurrentUserAdmin(),
            cancellationToken);

        if (result.NotFound)
        {
            return NotFound();
        }

        if (result.Forbidden)
        {
            return Forbid();
        }

        if (result.ErrorMessage != null)
        {
            return BadRequest(result.ErrorMessage);
        }

        return NoContent();
    }

    private bool TryGetCurrentUserId(out int userId)
    {
        var userIdClaim = User.FindFirstValue(ClaimTypes.NameIdentifier);
        return int.TryParse(userIdClaim, out userId);
    }

    private bool IsCurrentUserAdmin()
    {
        return User.IsInRole("Admin") || User.IsInRole("admin");
    }
}

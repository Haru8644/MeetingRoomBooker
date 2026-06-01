using System.Security.Claims;
using MeetingRoomBooker.Api.Contracts.RoomConflictRecords;
using MeetingRoomBooker.Api.Services.RoomConflictRecords;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

namespace MeetingRoomBooker.Api.Controllers;

[ApiController]
[Authorize]
[Route("api/room-conflict-records")]
public sealed class RoomConflictRecordsController : ControllerBase
{
    private readonly IRoomConflictRecordService _service;

    public RoomConflictRecordsController(IRoomConflictRecordService service)
    {
        _service = service;
    }

    [HttpGet]
    public async Task<ActionResult<IReadOnlyList<RoomConflictRecordResponse>>> GetRecords()
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var records = await _service.GetRecordsAsync(
            currentUserId,
            IsCurrentUserAdmin());

        return Ok(records);
    }

    [HttpGet("{id:int}")]
    public async Task<ActionResult<RoomConflictRecordResponse>> GetRecord(int id)
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var record = await _service.GetRecordAsync(
            id,
            currentUserId,
            IsCurrentUserAdmin());

        if (record == null)
        {
            return NotFound();
        }

        return Ok(record);
    }

    [HttpGet("summary")]
    public async Task<ActionResult<RoomConflictRecordSummaryResponse>> GetSummary()
    {
        var summary = await _service.GetSummaryAsync(DateTime.Now);
        return Ok(summary);
    }

    [HttpPost]
    public async Task<ActionResult<RoomConflictRecordResponse>> CreateRecord(
        [FromBody] CreateRoomConflictRecordRequest request)
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var result = await _service.CreateManualRecordAsync(
            request,
            currentUserId,
            IsCurrentUserAdmin());

        if (result.ErrorMessage != null)
        {
            return BadRequest(result.ErrorMessage);
        }

        return CreatedAtAction(
            nameof(GetRecord),
            new { id = result.Record!.Id },
            result.Record);
    }

    [HttpPut("{id:int}")]
    public async Task<ActionResult<RoomConflictRecordResponse>> UpdateRecord(
        int id,
        [FromBody] UpdateRoomConflictRecordRequest request)
    {
        if (!TryGetCurrentUserId(out var currentUserId))
        {
            return Unauthorized();
        }

        var result = await _service.UpdateRecordAsync(
            id,
            request,
            currentUserId,
            IsCurrentUserAdmin());

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

        return Ok(result.Record);
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
using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class AddWorkScheduleRecurrence : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "RepeatType",
                table: "WorkScheduleEntries",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<DateTime>(
                name: "RepeatUntil",
                table: "WorkScheduleEntries",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SeriesId",
                table: "WorkScheduleEntries",
                type: "TEXT",
                maxLength: 50,
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_WorkScheduleEntries_SeriesId",
                table: "WorkScheduleEntries",
                column: "SeriesId");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_WorkScheduleEntries_SeriesId",
                table: "WorkScheduleEntries");

            migrationBuilder.DropColumn(
                name: "RepeatType",
                table: "WorkScheduleEntries");

            migrationBuilder.DropColumn(
                name: "RepeatUntil",
                table: "WorkScheduleEntries");

            migrationBuilder.DropColumn(
                name: "SeriesId",
                table: "WorkScheduleEntries");
        }
    }
}

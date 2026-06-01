using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class AddRoomConflictRecords : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "RoomConflictRecords",
                columns: table => new
                {
                    Id = table.Column<int>(type: "INTEGER", nullable: false)
                        .Annotation("Sqlite:Autoincrement", true),
                    Type = table.Column<int>(type: "INTEGER", nullable: false),
                    Status = table.Column<int>(type: "INTEGER", nullable: false),
                    OccurredAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    RoomName = table.Column<string>(type: "TEXT", maxLength: 100, nullable: false),
                    ReservationIdA = table.Column<int>(type: "INTEGER", nullable: true),
                    ReservationIdB = table.Column<int>(type: "INTEGER", nullable: true),
                    Impact = table.Column<int>(type: "INTEGER", nullable: false),
                    Cause = table.Column<int>(type: "INTEGER", nullable: false),
                    Description = table.Column<string>(type: "TEXT", maxLength: 1000, nullable: false),
                    Resolution = table.Column<string>(type: "TEXT", maxLength: 1000, nullable: false),
                    DetectionKey = table.Column<string>(type: "TEXT", maxLength: 300, nullable: true),
                    ReportedByUserId = table.Column<int>(type: "INTEGER", nullable: true),
                    CreatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    UpdatedAt = table.Column<DateTime>(type: "TEXT", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_RoomConflictRecords", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_RoomConflictRecords_DetectionKey",
                table: "RoomConflictRecords",
                column: "DetectionKey",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_RoomConflictRecords_OccurredAt",
                table: "RoomConflictRecords",
                column: "OccurredAt");

            migrationBuilder.CreateIndex(
                name: "IX_RoomConflictRecords_RoomName",
                table: "RoomConflictRecords",
                column: "RoomName");

            migrationBuilder.CreateIndex(
                name: "IX_RoomConflictRecords_Status",
                table: "RoomConflictRecords",
                column: "Status");

            migrationBuilder.CreateIndex(
                name: "IX_RoomConflictRecords_Type",
                table: "RoomConflictRecords",
                column: "Type");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "RoomConflictRecords");
        }
    }
}

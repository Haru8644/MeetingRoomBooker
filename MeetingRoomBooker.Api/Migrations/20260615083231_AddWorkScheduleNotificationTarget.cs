using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class AddWorkScheduleNotificationTarget : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<int>(
                name: "TargetWorkScheduleEntryId",
                table: "Notifications",
                type: "INTEGER",
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_Notifications_UserId_Type_TargetReservationId",
                table: "Notifications",
                columns: new[] { "UserId", "Type", "TargetReservationId" });

            migrationBuilder.CreateIndex(
                name: "IX_Notifications_UserId_Type_TargetWorkScheduleEntryId",
                table: "Notifications",
                columns: new[] { "UserId", "Type", "TargetWorkScheduleEntryId" });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_Notifications_UserId_Type_TargetReservationId",
                table: "Notifications");

            migrationBuilder.DropIndex(
                name: "IX_Notifications_UserId_Type_TargetWorkScheduleEntryId",
                table: "Notifications");

            migrationBuilder.DropColumn(
                name: "TargetWorkScheduleEntryId",
                table: "Notifications");
        }
    }
}

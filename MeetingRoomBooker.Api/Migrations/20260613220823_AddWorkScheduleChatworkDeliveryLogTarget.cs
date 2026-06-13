using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class AddWorkScheduleChatworkDeliveryLogTarget : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<int>(
                name: "WorkScheduleEntryId",
                table: "ChatworkDeliveryLogs",
                type: "INTEGER",
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_ChatworkDeliveryLogs_DeliveryType",
                table: "ChatworkDeliveryLogs",
                column: "DeliveryType");

            migrationBuilder.CreateIndex(
                name: "IX_ChatworkDeliveryLogs_ReservationId",
                table: "ChatworkDeliveryLogs",
                column: "ReservationId");

            migrationBuilder.CreateIndex(
                name: "IX_ChatworkDeliveryLogs_TargetUserId",
                table: "ChatworkDeliveryLogs",
                column: "TargetUserId");

            migrationBuilder.CreateIndex(
                name: "IX_ChatworkDeliveryLogs_WorkScheduleEntryId",
                table: "ChatworkDeliveryLogs",
                column: "WorkScheduleEntryId");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_ChatworkDeliveryLogs_DeliveryType",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropIndex(
                name: "IX_ChatworkDeliveryLogs_ReservationId",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropIndex(
                name: "IX_ChatworkDeliveryLogs_TargetUserId",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropIndex(
                name: "IX_ChatworkDeliveryLogs_WorkScheduleEntryId",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "WorkScheduleEntryId",
                table: "ChatworkDeliveryLogs");
        }
    }
}

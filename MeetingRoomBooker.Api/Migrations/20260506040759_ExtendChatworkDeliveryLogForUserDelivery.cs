using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class ExtendChatworkDeliveryLogForUserDelivery : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_ChatworkDeliveryLogs_ReservationId_DeliveryType_ScheduledStartTime",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.AlterColumn<DateTime>(
                name: "SentAt",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(DateTime),
                oldType: "TEXT");

            migrationBuilder.AddColumn<DateTime>(
                name: "AttemptedAt",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                nullable: false,
                defaultValueSql: "CURRENT_TIMESTAMP");

            migrationBuilder.AddColumn<string>(
                name: "DeliveryKey",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                maxLength: 300,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "ErrorMessage",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                maxLength: 1000,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "RoomId",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                maxLength: 100,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Status",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                maxLength: 50,
                nullable: false,
                defaultValue: "Succeeded");

            migrationBuilder.AddColumn<int>(
                name: "TargetUserId",
                table: "ChatworkDeliveryLogs",
                type: "INTEGER",
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_ChatworkDeliveryLogs_DeliveryKey",
                table: "ChatworkDeliveryLogs",
                column: "DeliveryKey",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_ChatworkDeliveryLogs_DeliveryKey",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "AttemptedAt",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "DeliveryKey",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "ErrorMessage",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "RoomId",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "Status",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.DropColumn(
                name: "TargetUserId",
                table: "ChatworkDeliveryLogs");

            migrationBuilder.AlterColumn<DateTime>(
                name: "SentAt",
                table: "ChatworkDeliveryLogs",
                type: "TEXT",
                nullable: false,
                defaultValue: new DateTime(1, 1, 1, 0, 0, 0, 0, DateTimeKind.Unspecified),
                oldClrType: typeof(DateTime),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_ChatworkDeliveryLogs_ReservationId_DeliveryType_ScheduledStartTime",
                table: "ChatworkDeliveryLogs",
                columns: new[] { "ReservationId", "DeliveryType", "ScheduledStartTime" },
                unique: true);
        }
    }
}

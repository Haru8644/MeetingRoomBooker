using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class AddReservationSeriesId : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "SeriesId",
                table: "Reservations",
                type: "TEXT",
                maxLength: 50,
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_Reservations_SeriesId",
                table: "Reservations",
                column: "SeriesId");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_Reservations_SeriesId",
                table: "Reservations");

            migrationBuilder.DropColumn(
                name: "SeriesId",
                table: "Reservations");
        }
    }
}

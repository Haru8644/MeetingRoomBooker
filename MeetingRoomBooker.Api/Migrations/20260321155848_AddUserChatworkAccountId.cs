using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    public partial class AddUserChatworkAccountId : Migration
    {
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "ChatworkAccountId",
                table: "Users",
                type: "TEXT",
                maxLength: 100,
                nullable: true);
        }

        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "ChatworkAccountId",
                table: "Users");
        }
    }
}
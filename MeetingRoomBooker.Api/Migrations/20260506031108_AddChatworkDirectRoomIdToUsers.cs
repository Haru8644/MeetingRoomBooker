using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace MeetingRoomBooker.Api.Migrations
{
    /// <inheritdoc />
    public partial class AddChatworkDirectRoomIdToUsers : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterColumn<string>(
                name: "PasswordHash",
                table: "Users",
                type: "TEXT",
                maxLength: 512,
                nullable: true,
                oldClrType: typeof(string),
                oldType: "TEXT",
                oldMaxLength: 512);

            migrationBuilder.AlterColumn<bool>(
                name: "IsAdmin",
                table: "Users",
                type: "INTEGER",
                nullable: false,
                defaultValue: false,
                oldClrType: typeof(bool),
                oldType: "INTEGER");

            migrationBuilder.AddColumn<string>(
                name: "ChatworkDirectRoomId",
                table: "Users",
                type: "TEXT",
                maxLength: 100,
                nullable: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "ChatworkDirectRoomId",
                table: "Users");

            migrationBuilder.AlterColumn<string>(
                name: "PasswordHash",
                table: "Users",
                type: "TEXT",
                maxLength: 512,
                nullable: false,
                defaultValue: "",
                oldClrType: typeof(string),
                oldType: "TEXT",
                oldMaxLength: 512,
                oldNullable: true);

            migrationBuilder.AlterColumn<bool>(
                name: "IsAdmin",
                table: "Users",
                type: "INTEGER",
                nullable: false,
                oldClrType: typeof(bool),
                oldType: "INTEGER",
                oldDefaultValue: false);
        }
    }
}

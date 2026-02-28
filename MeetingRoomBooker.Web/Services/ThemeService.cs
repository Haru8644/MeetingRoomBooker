using System;

namespace MeetingRoomBooker.Services
{
    public class ThemeService
    {
        public string CurrentTheme { get; private set; } = "dark";
        public event Action? OnThemeChanged;

        public void SetTheme(string theme)
        {
            CurrentTheme = theme;
            OnThemeChanged?.Invoke();
        }
    }
}
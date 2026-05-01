package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SetAutoStart registers or unregisters the application for auto-start on login.
func SetAutoStart(enabled bool) error {
	switch runtime.GOOS {
	case "darwin":
		return setAutoStartDarwin(enabled)
	case "linux":
		return setAutoStartLinux(enabled)
	case "windows":
		return setAutoStartWindows(enabled)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// IsAutoStartEnabled checks if auto-start is currently configured.
func IsAutoStartEnabled() bool {
	switch runtime.GOOS {
	case "darwin":
		plist := launchAgentPath()
		_, err := os.Stat(plist)
		return err == nil
	case "linux":
		desktop := autostartDesktopPath()
		_, err := os.Stat(desktop)
		return err == nil
	default:
		return false
	}
}

// --- macOS: LaunchAgent plist ---

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.ai-flight-dashboard.plist")
}

func setAutoStartDarwin(enabled bool) error {
	plistPath := launchAgentPath()

	if !enabled {
		return os.Remove(plistPath)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ai-flight-dashboard</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--gui</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`, exePath)

	dir := filepath.Dir(plistPath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(plistPath, []byte(plistContent), 0644)
}

// --- Linux: XDG Autostart ---

func autostartDesktopPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "ai-flight-dashboard.desktop")
}

func setAutoStartLinux(enabled bool) error {
	desktopPath := autostartDesktopPath()

	if !enabled {
		return os.Remove(desktopPath)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=AI Flight Dashboard
Exec=%s --gui
Icon=ai-flight-dashboard
Terminal=false
X-GNOME-Autostart-enabled=true
`, exePath)

	dir := filepath.Dir(desktopPath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(desktopPath, []byte(content), 0644)
}

// --- Windows: Registry (stub — requires golang.org/x/sys/windows) ---

func setAutoStartWindows(enabled bool) error {
	// Windows autostart via registry requires the golang.org/x/sys/windows package.
	// For now, this is a placeholder — will be implemented when Windows CI is set up.
	return fmt.Errorf("windows autostart: not yet implemented (requires registry access)")
}

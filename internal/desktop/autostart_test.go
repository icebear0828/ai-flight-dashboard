package desktop

import (
	"strings"
	"testing"
)

func TestDarwinAutoStartPlistUsesExplicitDataDir(t *testing.T) {
	content := darwinAutoStartPlist("/Applications/AI Flight Dashboard.app/Contents/MacOS/ai-flight-dashboard", "/Users/c/.ai-flight-dashboard")

	if strings.Contains(content, "--gui") {
		t.Fatalf("autostart plist must not include unsupported --gui flag: %s", content)
	}
	for _, want := range []string{
		"<string>--data-dir</string>",
		"<string>/Users/c/.ai-flight-dashboard</string>",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("autostart plist missing %q: %s", want, content)
		}
	}
}

func TestLinuxAutoStartDesktopEntryUsesExplicitDataDir(t *testing.T) {
	content := linuxAutoStartDesktopEntry("/usr/local/bin/ai-flight-dashboard", "/home/c/.ai-flight-dashboard")

	if strings.Contains(content, "--gui") {
		t.Fatalf("autostart desktop entry must not include unsupported --gui flag: %s", content)
	}
	if !strings.Contains(content, "Exec=/usr/local/bin/ai-flight-dashboard --data-dir /home/c/.ai-flight-dashboard") {
		t.Fatalf("autostart desktop entry missing explicit data-dir: %s", content)
	}
}

package main

import (
	"ai-flight-dashboard/internal/desktop"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestConfigureGUIWindowLifecycleHidesWindowOnClose(t *testing.T) {
	appOptions := &options.App{}
	app := desktop.NewApp(nil, nil)

	configureGUIWindowLifecycle(appOptions, app)

	if !appOptions.HideWindowOnClose {
		t.Fatal("expected GUI close button to hide the window instead of quitting")
	}
}

func TestConfigureGUIWindowLifecycleRestoresHiddenWindowOnSecondLaunch(t *testing.T) {
	appOptions := &options.App{}
	app := desktop.NewApp(nil, nil)

	configureGUIWindowLifecycle(appOptions, app)

	if appOptions.SingleInstanceLock == nil {
		t.Fatal("expected single-instance lock to be configured")
	}
	if appOptions.SingleInstanceLock.UniqueId != "ai-flight-dashboard" {
		t.Fatalf("unexpected single-instance ID: %q", appOptions.SingleInstanceLock.UniqueId)
	}
	if appOptions.SingleInstanceLock.OnSecondInstanceLaunch == nil {
		t.Fatal("expected second-launch callback to restore hidden window")
	}
}

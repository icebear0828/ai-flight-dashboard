package main

import (
	"ai-flight-dashboard/internal/desktop"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func configureGUIWindowLifecycle(appOptions *options.App, app *desktop.App) {
	appOptions.HideWindowOnClose = true
	appOptions.SingleInstanceLock = &options.SingleInstanceLock{
		UniqueId: "ai-flight-dashboard",
		OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
			if app.GetContext() == nil {
				return
			}
			app.ShowWindow()
		},
	}
}

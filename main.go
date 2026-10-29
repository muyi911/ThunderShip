package main

import (
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := NewApp()
	a.dir, a.initErr = prepareDataDirectory()
	windows := application.WindowsOptions{}
	if a.initErr == nil {
		windows.WebviewUserDataPath = filepath.Join(a.dir, "webview")
	}
	desktop := application.New(application.Options{
		Name:     "ThunderShip",
		Windows:  windows,
		Services: []application.Service{application.NewService(a)},
		Assets:   application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "8af76964-73a0-476d-93cf-d7635829cf86",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				a.window.UnMinimise()
				a.window.Show()
				a.window.Focus()
			},
		},
	})
	a.desktop = desktop
	a.window = desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "霆舟 ThunderShip", Width: 1480, Height: 940, MinWidth: 1050, MinHeight: 700,
		Frameless: true, EnableFileDrop: true, URL: "/",
		BackgroundColour: application.RGBA{Red: 237, Green: 239, Blue: 242, Alpha: 255},
	})
	a.window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		details := event.Context().DropTargetDetails()
		if details != nil {
			desktop.Event.Emit("files-dropped", map[string]any{
				"paths": event.Context().DroppedFiles(), "x": details.X, "y": details.Y,
			})
		}
	})
	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}

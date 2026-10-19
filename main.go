package main

import (
	"embed"
	"io/fs"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The frontend is plain ES modules served straight from the embedded
// filesystem, so no Node toolchain is needed to build the app.
//
//go:embed all:frontend
var embeddedFrontend embed.FS

func main() {
	frontend, err := fs.Sub(embeddedFrontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	service := NewAppService()

	app := application.New(application.Options{
		Name:        "PicPress",
		Description: "拖进来，就变小",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	service.attach(app)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "PicPress",
		Width:            1120,
		Height:           760,
		MinWidth:         900,
		MinHeight:        600,
		URL:              "/",
		BackgroundColour: application.NewRGB(246, 245, 242),
		EnableFileDrop:   true,
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 56,
			Backdrop:                application.MacBackdropNormal,
		},
	})

	// The native drag & drop API gives us real file paths. The frontend is
	// asked to register them so the UI stays the single source of truth.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		app.Event.Emit("files-dropped", files)
	})

	app.OnShutdown(service.Cleanup)

	setupMenu(app)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupMenu(app *application.App) {
	menu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}

	fileMenu := menu.AddSubmenu("文件")
	fileMenu.Add("打开图片…").
		SetAccelerator("CmdOrCtrl+O").
		OnClick(func(*application.Context) {
			app.Event.Emit("menu:open")
		})
	fileMenu.Add("保存全部").
		SetAccelerator("CmdOrCtrl+S").
		OnClick(func(*application.Context) {
			app.Event.Emit("menu:save-all")
		})
	fileMenu.AddSeparator()
	fileMenu.Add("清空列表").
		SetAccelerator("CmdOrCtrl+K").
		OnClick(func(*application.Context) {
			app.Event.Emit("menu:clear")
		})

	menu.AddRole(application.EditMenu)
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.WindowMenu)
	}
	menu.AddRole(application.HelpMenu)

	app.Menu.Set(menu)
}

package main

import (
	"embed"
	"io/fs"
	"log"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"picpress/internal/push"
)

// The Vite build output is embedded into the binary. During development
// Wails proxies to FRONTEND_DEVSERVER_URL instead of using these assets.
//
//go:embed all:frontend/dist
var embeddedFrontend embed.FS

func main() {
	frontend, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	service := NewAppService()
	pushService := NewPushService()

	app := application.New(application.Options{
		Name:        "PicPress",
		Description: "拖进来，就变小",
		Services: []application.Service{
			application.NewService(service),
			application.NewService(pushService),
		},
		FileAssociations: []string{push.ConfigExt},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	service.attach(app)
	pushService.attach(app)

	// Double-clicking a .picpress-config (file association) opens it here.
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(event *application.ApplicationEvent) {
		if name := event.Context().Filename(); strings.HasSuffix(strings.ToLower(name), push.ConfigExt) {
			_, _ = pushService.ImportConfig(name)
		}
	})

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

	// The native drag & drop API gives us real file paths. A dropped config is
	// imported; with an active connection the rest goes to the push prepare
	// panel; otherwise the frontend adds them to the local compress session.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		var configs, files []string
		for _, f := range event.Context().DroppedFiles() {
			if strings.HasSuffix(strings.ToLower(f), push.ConfigExt) {
				configs = append(configs, f)
			} else {
				files = append(files, f)
			}
		}
		if len(configs) > 0 {
			if _, err := pushService.ImportConfig(configs[len(configs)-1]); err != nil {
				app.Event.Emit("push:error", err.Error())
			}
		}
		if len(files) == 0 {
			return
		}
		if pushService.HasConnection() {
			if _, err := pushService.Prepare(files); err != nil {
				app.Event.Emit("push:error", err.Error())
			}
			return
		}
		app.Event.Emit("files-dropped", files)
	})

	app.OnShutdown(func() {
		pushService.shutdown()
		service.Cleanup()
	})

	setupMenu(app, pushService)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupMenu(app *application.App, pushService *PushService) {
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
	fileMenu.Add("导入配置…").
		SetAccelerator("CmdOrCtrl+I").
		OnClick(func(*application.Context) {
			go func() {
				if _, err := pushService.PickConfig(); err != nil {
					app.Event.Emit("push:error", err.Error())
				}
			}()
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

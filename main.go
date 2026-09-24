//go:build !bindings

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/service"
	"github.com/gemsnote/gemsnote/webapi"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func getDataPath() string {
	homeDir, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "gemsnote")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		return filepath.Join(appData, "gemsnote")
	default:
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(homeDir, ".config")
		}
		return filepath.Join(configDir, "gemsnote")
	}
}

// migrateLegacyData silently adopts the pre-rename "leanote" data directory so upgrades keep their local data.
func migrateLegacyData(dataPath string) {
	legacy := filepath.Join(filepath.Dir(dataPath), "leanote")
	if _, err := os.Stat(filepath.Join(dataPath, "gemsnote.db")); err == nil {
		return
	}
	if _, err := os.Stat(filepath.Join(legacy, "leanote.db")); err != nil {
		return
	}
	_ = copyDir(legacy, dataPath)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

func main() {
	dataPath := getDataPath()
	os.MkdirAll(dataPath, 0755)
	migrateLegacyData(dataPath)
	dbPath := filepath.Join(dataPath, "gemsnote.db")

	database, err := db.New(dbPath)
	if err != nil {
		println("Failed to initialize database:", err.Error())
		os.Exit(1)
	}

	app := NewApp(database)
	app.sharedSync.OnRevocation = func(noteIDs []string) {
		if app.ctx != nil {
			wailsruntime.EventsEmit(app.ctx, "shared-notes-revoked", noteIDs)
		}
	}
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		println("Failed to access embedded frontend:", err.Error())
		os.Exit(1)
	}

	serverProxy := webapi.NewServerProxy(database, app.files)
	apiHandler := &webapi.Handler{
		DB:            database,
		Files:         service.NewFileService(database),
		Proxy:         serverProxy,
		Version:       AppVersion,
		Dist:          dist,
		OnBeforeLogin: app.sync.StopBackgroundDownloads,
		OnLogin: func(hasLocalCache bool) (any, error) {
			if hasLocalCache {
				app.SetAutoSyncPaused(true)
				return map[string]interface{}{"SyncChoiceRequired": true}, nil
			}
			app.SetAutoSyncPaused(true)
			// Return authentication immediately. The frontend starts ResetSync as
			// a separate request so it can distinguish "signing in" from the
			// potentially long initial download and display accurate progress.
			return map[string]interface{}{"ResetSyncRequired": true}, nil
		},
		OnSessionChanged: app.SetSessionLive,
		OnSyncProgress:   app.GetSyncProgress,
		OnWaitForImage:   app.sync.WaitForBackgroundImage,
		OnDownloadStatus: app.sync.GetDownloadStatus,
		OnInitialSync: func() (any, error) {
			return app.InitialSync(), nil
		},
		OnLogout: func() error {
			user, _ := database.GetActiveUser()
			if user == nil || user.IsLocal || user.Host == "" || user.Token == "" {
				return nil
			}
			pending, err := database.HasPendingChanges(user.ID)
			if err != nil {
				return err
			}
			if !pending {
				return nil
			}
			syncErr := app.runFullSync(false)
			// "Pending" only means local changes that still need uploading. A
			// later pull/avatar/attachment failure must not prevent logout once
			// every local dirty row has already been accepted by the server.
			pending, pendingErr := database.HasPendingChanges(user.ID)
			if pendingErr != nil {
				return pendingErr
			}
			if !pending {
				return nil
			}
			if syncErr == nil {
				return fmt.Errorf("local changes remain pending")
			}
			return syncErr
		},
		OnSync: func() (any, error) {
			// User profile/avatar refresh is not part of incremental note sync.
			// A missing avatar must never prevent pending edits from uploading.
			app.SetAutoSyncPaused(false)
			result := app.IncrSync()
			return result, nil
		},
		OnFullSync: func() (any, error) {
			// Refresh profile information on full sync, but keep personal data
			// synchronization available if the profile/avatar request fails.
			app.SetAutoSyncPaused(false)
			serverProxy.RefreshUserProfile()
			result := app.FullSyncForce()
			return result, nil
		},
		OnResetSync: func() (any, error) {
			return app.ResetSync(), nil
		},
		OnSharedDownload: func() { go app.sharedSync.DownloadPending() },
	}

	err = wails.Run(&options.App{
		// Stay above the shared UI's 1100px compact breakpoint so all three
		// columns are visible on startup; smaller windows remain resizable.
		Title:     "Gemsnote 珠玑笔记",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 500,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: apiHandler,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.gemsnote.desktop",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				app.ShowWindow()
				if len(secondInstanceData.Args) > 1 {
					deepLink := secondInstanceData.Args[1]
					if deepLink != "" {
						app.HandleDeepLink(deepLink)
					}
				}
			},
		},
		Linux: &linux.Options{
			ProgramName: "gemsnote",
			Icon:        appIcon,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

func (a *App) startAutoSync() {
	go func() {
		if user, _ := a.db.GetActiveUser(); user != nil && user.Token != "" && a.CanAutoSync() {
			a.IncrSync()
			a.sharedSync.SyncOnce()
		}
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		sharedTicker := time.NewTicker(5 * time.Minute)
		defer sharedTicker.Stop()
		for {
			select {
			case <-ticker.C:
				if user, _ := a.db.GetActiveUser(); user != nil && user.Token != "" && !a.IsSyncing() && a.CanAutoSync() {
					a.IncrSync()
				}
			case <-sharedTicker.C:
				if user, _ := a.db.GetActiveUser(); user != nil && user.Token != "" && a.CanAutoSync() {
					a.sharedSync.SyncOnce()
				}
			}
		}
	}()
}

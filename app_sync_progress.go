package main

import "github.com/gemsnote/gemsnote/models"

func (a *App) GetSyncProgress() models.SyncProgress {
	a.syncProgressMu.RLock()
	defer a.syncProgressMu.RUnlock()
	return a.syncProgress
}

func (a *App) beginSyncProgress(mode string) {
	a.syncProgressMu.Lock()
	a.syncProgress = models.SyncProgress{Running: true, Mode: mode, Stage: "checking"}
	a.syncProgressMu.Unlock()
}

func (a *App) finishSyncProgress() {
	a.syncProgressMu.Lock()
	a.syncProgress.Running = false
	a.syncProgressMu.Unlock()
}

func (a *App) recordSyncProgress(progress models.SyncProgress) models.SyncProgress {
	a.syncProgressMu.Lock()
	defer a.syncProgressMu.Unlock()
	progress.Mode = a.syncProgress.Mode
	progress.Running = a.syncProgress.Running
	a.syncProgress = progress
	return progress
}

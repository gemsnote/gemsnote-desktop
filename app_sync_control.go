package main

import (
	"context"
	"errors"

	syncservice "github.com/gemsnote/gemsnote/sync"
)

type syncOperation struct {
	id     uint64
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Registration and session shutdown share a lock: no old-session operation
// can start between cancellation and switching the active database account.
func (a *App) beginSyncOperation(mode string) (*syncOperation, error) {
	a.syncControlMu.Lock()
	defer a.syncControlMu.Unlock()
	if a.syncBlocked {
		return nil, context.Canceled
	}
	if !a.syncRunMu.TryLock() {
		return nil, syncservice.ErrAlreadySyncing
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.syncSequence++
	op := &syncOperation{id: a.syncSequence, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	a.syncOperation = op
	a.sync.SetContext(ctx)
	a.beginSyncProgress(mode)
	a.syncProgressMu.Lock()
	a.syncProgress.RunID = op.id
	a.syncProgressMu.Unlock()
	return op, nil
}

func (a *App) endSyncOperation(op *syncOperation) bool {
	a.syncControlMu.Lock()
	defer a.syncControlMu.Unlock()
	cancelled := op.ctx.Err() != nil
	if cancelled {
		a.SetAutoSyncPaused(true)
		a.sync.StopBackgroundDownloads()
	}
	op.cancel()
	a.finishSyncProgress()
	a.syncOperation = nil
	a.syncRunMu.Unlock()
	close(op.done)
	return cancelled
}

// CancelSync waits for the specific operation's writers to stop. A delayed
// cancel from an earlier dialog must never cancel a newer sync.
func (a *App) CancelSync(runID uint64) map[string]interface{} {
	a.syncControlMu.Lock()
	op := a.syncOperation
	if op == nil || runID == 0 || op.id != runID {
		a.syncControlMu.Unlock()
		return map[string]interface{}{"Ok": true, "Cancelled": false}
	}
	a.SetAutoSyncPaused(true)
	op.cancel()
	a.syncControlMu.Unlock()
	<-op.done
	return map[string]interface{}{"Ok": true, "Cancelled": true}
}

func (a *App) stopSessionSync() {
	a.sessionLive.Store(false)
	a.syncControlMu.Lock()
	a.syncBlocked = true
	op := a.syncOperation
	if op != nil {
		op.cancel()
	}
	a.syncControlMu.Unlock()
	if op != nil {
		<-op.done
	}
	a.sync.StopBackgroundDownloads()
	a.sharedSync.SuspendAndWait()
}

func syncFailure(err error) map[string]interface{} {
	if errors.Is(err, context.Canceled) {
		return map[string]interface{}{"Ok": true, "Cancelled": true}
	}
	if errors.Is(err, syncservice.ErrAlreadySyncing) {
		return map[string]interface{}{"Ok": true, "AlreadyRunning": true}
	}
	return map[string]interface{}{"Ok": false, "Msg": err.Error()}
}

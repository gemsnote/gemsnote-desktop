package sync

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/service"
)

type SyncService struct {
	db       *db.Database
	api      *api.Client
	files    *service.FileService
	maxEntry int

	mu            sync.Mutex
	isSyncing     bool
	needSyncAgain bool
	retryCount    int
	progressCb    func(stage string, current, total int)
}

func NewSyncService(database *db.Database, client *api.Client) *SyncService {
	return &SyncService{
		db:       database,
		api:      client,
		files:    service.NewFileService(database),
		maxEntry: 200,
	}
}

func (s *SyncService) SetProgressCallback(cb func(stage string, current, total int)) {
	s.progressCb = cb
}

func (s *SyncService) emitProgress(stage string, current, total int) {
	if s.progressCb != nil {
		s.progressCb(stage, current, total)
	}
}

func (s *SyncService) FullSync() (*models.SyncInfo, error) {
	s.mu.Lock()
	if s.isSyncing {
		s.mu.Unlock()
		return nil, ErrAlreadySyncing
	}
	s.isSyncing = true
	s.needSyncAgain = false
	s.retryCount = 0
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
	}()

	logrus.Info("Starting full sync...")
	s.emitProgress("start", 0, 100)

	syncInfo := models.NewSyncInfo()

	user, err := s.db.GetActiveUser()
	if err != nil || user == nil {
		return nil, err
	}

	s.db.SetCurrentUser(user.ID)
	s.api.SetHost(user.Host)
	s.api.SetToken(user.Token)
	if err := s.restoreLostServerMappings(user.ID); err != nil {
		return nil, err
	}

	// Always protect local work first. Pulling a remote snapshot can update or
	// reconcile the same rows, so every dirty row that already existed when the
	// sync started must be accepted by the server before any download begins.
	s.emitProgress("push", 0, 100)
	if err := s.sendChanges(syncInfo); err != nil {
		logrus.Errorf("Send changes error: %v", err)
		return nil, err
	}

	serverState, err := s.api.GetLastSyncState()
	if err != nil {
		return nil, err
	}

	s.emitProgress("notebooks", 20, 100)
	if err := s.syncNotebooks(-1, syncInfo); err != nil {
		logrus.Errorf("Sync notebooks error: %v", err)
		return nil, err
	}

	s.emitProgress("notes", 40, 100)
	if err := s.syncNotes(-1, syncInfo); err != nil {
		logrus.Errorf("Sync notes error: %v", err)
		return nil, err
	}

	s.emitProgress("tags", 60, 100)
	if err := s.syncTags(-1, syncInfo); err != nil {
		logrus.Errorf("Sync tags error: %v", err)
		return nil, err
	}

	// A complete snapshot can discover clean cache rows that no longer exist
	// on the server and requeue them as new local data. Upload only those newly
	// generated dirty rows after the download/merge pass.
	s.emitProgress("push", 75, 100)
	if err := s.sendChanges(syncInfo); err != nil {
		logrus.Errorf("Send changes error: %v", err)
		return nil, err
	}

	s.emitProgress("images", 85, 100)
	if err := s.syncImagesAndAttachs(syncInfo); err != nil {
		logrus.Errorf("Sync images/attachs error: %v", err)
		return nil, err
	}
	if merged, err := s.db.CountVisibleNotes(user.ID); err == nil {
		logrus.Infof("Full sync merged notes available locally: %d", merged)
	}

	if err := s.db.UpdateUserSyncState(user.ID, map[string]int64{
		"last_sync_usn":  serverState.LastSyncUsn,
		"last_sync_time": time.Now().Unix(),
	}); err != nil {
		return nil, err
	}

	s.emitProgress("done", 100, 100)
	logrus.Info("Full sync completed")
	return syncInfo, nil
}

func (s *SyncService) IncrSync() (*models.SyncInfo, error) {
	s.mu.Lock()
	if s.isSyncing {
		s.mu.Unlock()
		return nil, ErrAlreadySyncing
	}
	s.isSyncing = true
	s.needSyncAgain = false
	s.retryCount = 0
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
	}()

	logrus.Info("Starting incremental sync...")

	syncInfo := models.NewSyncInfo()

	user, err := s.db.GetActiveUser()
	if err != nil || user == nil {
		return nil, err
	}

	s.db.SetCurrentUser(user.ID)
	s.api.SetHost(user.Host)
	s.api.SetToken(user.Token)
	if err := s.restoreLostServerMappings(user.ID); err != nil {
		return nil, err
	}

	lastUsn, _, _, _, err := s.db.GetAllLastSyncState(user.ID)
	if err != nil {
		return nil, err
	}

	// Upload the dirty set captured at the start before applying any remote
	// changes to the local cache.
	if err := s.sendChanges(syncInfo); err != nil {
		logrus.Errorf("Send changes error: %v", err)
		return nil, err
	}

	serverState, err := s.api.GetLastSyncState()
	if err != nil {
		return nil, err
	}
	if serverState.LastSyncUsn < lastUsn {
		logrus.Warnf("Server sync cursor moved backwards (%d -> %d); reconciling full snapshot", lastUsn, serverState.LastSyncUsn)
		if err := s.syncNotebooks(-1, syncInfo); err != nil {
			return nil, err
		}
		if err := s.syncNotes(-1, syncInfo); err != nil {
			return nil, err
		}
		if err := s.syncTags(-1, syncInfo); err != nil {
			return nil, err
		}
		if err := s.db.UpdateUserSyncState(user.ID, map[string]int64{"last_sync_usn": serverState.LastSyncUsn}); err != nil {
			return nil, err
		}
	} else if serverState.LastSyncUsn > lastUsn {
		logrus.Debugf("Server has updates, pulling...")

		if err := s.syncNotebooks(lastUsn, syncInfo); err != nil {
			logrus.Errorf("Sync notebooks error: %v", err)
			return nil, err
		}

		if err := s.syncNotes(lastUsn, syncInfo); err != nil {
			logrus.Errorf("Sync notes error: %v", err)
			return nil, err
		}

		if err := s.syncTags(lastUsn, syncInfo); err != nil {
			logrus.Errorf("Sync tags error: %v", err)
			return nil, err
		}
	}

	// Pull/merge can create conflict copies or requeue cache rows. Give those
	// newly-created local changes a second upload pass without changing the
	// guarantee that the original dirty set was uploaded before downloading.
	if err := s.sendChanges(syncInfo); err != nil {
		logrus.Errorf("Send changes error: %v", err)
		return nil, err
	}

	if err := s.syncImagesAndAttachs(syncInfo); err != nil {
		logrus.Errorf("Sync images/attachs error: %v", err)
		return nil, err
	}

	logrus.Info("Incremental sync completed")
	return syncInfo, nil
}

func (s *SyncService) restoreLostServerMappings(userID string) error {
	if err := s.db.RestoreLostServerNotebookMappings(userID); err != nil {
		return err
	}
	return s.db.RestoreLostServerNoteMappings(userID)
}

func (s *SyncService) ForceFullSync() error {
	user, err := s.db.GetActiveUser()
	if err != nil || user == nil {
		return err
	}

	s.db.UpdateUserSyncState(user.ID, map[string]int64{
		"last_sync_usn": -1,
		"notebook_usn":  -1,
		"note_usn":      -1,
		"tag_usn":       -1,
	})

	_, err = s.FullSync()
	return err
}

func (s *SyncService) IsSyncing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSyncing
}

func (s *SyncService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needSyncAgain = false
}

var ErrAlreadySyncing = &SyncError{Msg: "already syncing"}

type SyncError struct {
	Msg string
}

func (e *SyncError) Error() string {
	return e.Msg
}

func (s *SyncService) checkNeedSyncAgain(usn int64) {
	user, _ := s.db.GetActiveUser()
	if user == nil {
		return
	}

	lastUsn, _, _, _, _ := s.db.GetAllLastSyncState(user.ID)

	if usn != lastUsn+1 {
		s.mu.Lock()
		s.needSyncAgain = true
		s.mu.Unlock()
	} else {
		s.db.UpdateLastSyncUsn(user.ID, usn)
	}
}

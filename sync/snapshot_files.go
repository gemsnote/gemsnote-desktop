package sync

import (
	"context"
	"fmt"
	"os"
	"sort"
	stdsync "sync"
	"sync/atomic"
	"time"

	"github.com/gemsnote/gemsnote/api"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/utils"
	"github.com/sirupsen/logrus"
)

const snapshotDownloadWorkers = 4

type snapshotFile struct {
	id, noteID, title string
	attachment        bool
}

type snapshotDownloads struct {
	cancel       context.CancelFunc
	done         chan struct{}
	images       map[string]chan struct{}
	completed    atomic.Int32
	failed       atomic.Int32
	profileReady atomic.Bool
	total        int
}

func (s *SyncService) GetDownloadStatus() models.DownloadStatus {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	batch := s.downloads
	if batch == nil {
		return models.DownloadStatus{}
	}
	running := true
	select {
	case <-batch.done:
		running = false
	default:
	}
	return models.DownloadStatus{Running: running, Completed: batch.completed.Load(), Total: batch.total, Failed: batch.failed.Load(), ProfileReady: batch.profileReady.Load()}
}

// Stop joins all writers before a logout, account switch, reset or shutdown.
func (s *SyncService) StopBackgroundDownloads() {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	s.stopDownloadsLocked()
}

func (s *SyncService) stopDownloadsLocked() {
	if s.downloads != nil {
		s.downloads.cancel()
		<-s.downloads.done
		s.downloads = nil
	}
}

// Let an image opened immediately after sync finish loading naturally, rather
// than caching a 404 in the WebView before its queued download has finished.
func (s *SyncService) WaitForBackgroundImage(ctx context.Context, fileID string) {
	s.downloadMu.Lock()
	var ready <-chan struct{}
	if s.downloads != nil {
		ready = s.downloads.images[fileID]
	}
	s.downloadMu.Unlock()
	if ready == nil {
		return
	}
	select {
	case <-ready:
	case <-ctx.Done():
	}
}

func (s *SyncService) startSnapshotDownloads(user *models.User) {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	s.stopDownloadsLocked()
	ctx, cancel := context.WithCancel(context.Background())
	batch := &snapshotDownloads{cancel: cancel, done: make(chan struct{}), images: make(map[string]chan struct{})}
	files := make(map[string]snapshotFile, len(s.snapshotFiles))
	for key, job := range s.snapshotFiles {
		files[key] = job
		if !job.attachment {
			batch.images[job.id] = make(chan struct{})
		}
	}
	s.downloads = batch
	batch.total = len(files)
	// Do not share the foreground client's mutable host/token with a job
	// that can outlive the next incremental sync.
	client := api.NewClient()
	client.SetHost(user.Host)
	client.SetToken(user.Token)
	account := *user
	imageDir, attachDir := s.files.GetUserImageDir(user.ID), s.files.GetUserAttachDir(user.ID)
	go func() {
		defer close(batch.done)
		defer cancel()
		defer func() {
			for _, ready := range batch.images {
				select {
				case <-ready:
				default:
					close(ready)
				}
			}
		}()
		var profile stdsync.WaitGroup
		if s.BackgroundProfileRefresh != nil {
			profile.Add(1)
			go func() {
				defer profile.Done()
				defer batch.profileReady.Store(true)
				s.BackgroundProfileRefresh(ctx, account)
			}()
		}
		s.downloadSnapshotFiles(ctx, client, account.ID, files, imageDir, attachDir, batch)
		profile.Wait()
	}()
}

// Deduplicate across the whole snapshot, including failed resources: one
// broken image referenced by many notes must not incur a timeout per note.
func (s *SyncService) queueSnapshotFiles(note *models.Note) error {
	if s.snapshotFiles == nil || note.IsDeleted {
		return nil
	}
	attachments := make(map[string]bool)
	for _, file := range note.Files {
		if file == nil || file.FileID == "" {
			continue
		}
		job := snapshotFile{id: file.FileID, title: file.Title, attachment: file.IsAttach}
		if job.attachment {
			remoteID := note.NoteID
			if note.ServerNoteID != "" {
				remoteID = note.ServerNoteID
			}
			var err error
			job.noteID, err = s.db.GetLocalNoteID(remoteID)
			if err != nil {
				return err
			}
			if job.noteID == "" {
				return fmt.Errorf("missing local note mapping for snapshot attachment")
			}
			attachments[file.FileID] = true
		}
		key := "image:" + job.id
		if job.attachment {
			key = "attach:" + job.id + ":" + job.noteID
		}
		s.snapshotFiles[key] = job
	}
	for _, match := range imageFileIDRe.FindAllStringSubmatch(note.Content, -1) {
		id := match[1]
		if !attachments[id] {
			s.snapshotFiles["image:"+id] = snapshotFile{id: id}
		}
	}
	return nil
}

// Network/file writes run in a bounded pool. SQLite writes remain serial;
// this background phase never changes the note-sync modal or dirty status.
func (s *SyncService) downloadSnapshotFiles(ctx context.Context, client *api.Client, userID string, files map[string]snapshotFile, imageDir, attachDir string, batch *snapshotDownloads) {
	started := time.Now()
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return
	}
	type result struct {
		job         snapshotFile
		path, title string
		err         error
	}
	jobs := make(chan snapshotFile, len(keys))
	results := make(chan result, snapshotDownloadWorkers)
	for _, key := range keys {
		jobs <- files[key]
	}
	close(jobs)
	var workers stdsync.WaitGroup
	for i := 0; i < snapshotDownloadWorkers && i < len(keys); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := result{job: job}
				if job.attachment {
					r.path, r.title, r.err = client.GetAttachContext(ctx, job.id, attachDir)
				} else {
					r.path, r.err = client.GetImageContext(ctx, job.id, imageDir)
				}
				results <- r
			}
		}()
	}
	go func() { workers.Wait(); close(results) }()
	completed, failed := 0, 0
	logrus.Infof("Background media downloads started: total=%d workers=%d", len(keys), snapshotDownloadWorkers)
	for r := range results {
		if ctx.Err() != nil {
			if r.path != "" {
				_ = os.Remove(r.path)
			}
			continue
		}
		completed++
		if r.err != nil {
			failed++
			logrus.Warnf("Snapshot file download failed: id=%s error=%v", r.job.id, r.err)
		} else {
			now := time.Now()
			var err error
			if r.job.attachment {
				if r.job.title != "" {
					r.title = r.job.title
				}
				err = s.db.InsertAttach(&models.Attach{ID: utils.ObjectId(), FileID: utils.ObjectId(), ServerFileID: r.job.id, NoteID: r.job.noteID, UserID: userID, Title: r.title, Path: r.path, IsAttach: true, CreatedTime: &now})
			} else {
				err = s.db.InsertImage(&models.Image{ID: utils.ObjectId(), FileID: r.job.id, ServerFileID: r.job.id, UserID: userID, Path: r.path, CreatedTime: &now})
			}
			if err != nil {
				failed++
				_ = os.Remove(r.path)
				logrus.Warnf("Background media persistence failed: id=%s error=%v", r.job.id, err)
			}
		}
		batch.completed.Store(int32(completed))
		batch.failed.Store(int32(failed))
		if !r.job.attachment {
			// A second close is avoided by keeping the batch map immutable and
			// using a nonblocking receive to detect an already closed channel.
			ready := batch.images[r.job.id]
			select {
			case <-ready:
			default:
				close(ready)
			}
		}
	}
	batch.completed.Store(int32(completed))
	batch.failed.Store(int32(failed))
	logrus.Infof("Background media downloads finished: completed=%d total=%d failed=%d canceled=%t elapsed=%s", completed, len(keys), failed, ctx.Err() != nil, time.Since(started).Round(time.Millisecond))
}

package models

// Percent is overall progress. Current/Total are item counts in the current
// phase; Total=0 means the server has not provided an exact item count yet.
type SyncProgress struct {
	Running bool
	Mode    string
	Stage   string
	Percent int
	Current int
	Total   int
}

type DownloadStatus struct {
	Running      bool
	Completed    int32
	Total        int
	Failed       int32
	ProfileReady bool
}

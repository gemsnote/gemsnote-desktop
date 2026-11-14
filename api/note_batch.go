package api

import (
	"encoding/json"
	"fmt"
	"github.com/gemsnote/gemsnote/models"
	"net/http"
	"time"
)

const NoteMetadataPageSize = 500
const NoteContentBatchSize = 20

// These POSTs are read-only and safe to retry. Never apply this retry policy
// to note writes, whose response may be lost after the write is committed.
func (c *Client) GetNoteContents(ids []string) ([]*models.Note, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > NoteContentBatchSize {
		var all []*models.Note
		for start := 0; start < len(ids); start += NoteContentBatchSize {
			part, err := c.GetNoteContents(ids[start:min(start+NoteContentBatchSize, len(ids))])
			if err != nil {
				return nil, err
			}
			all = append(all, part...)
		}
		return all, nil
	}
	var body []byte
	var status int
	var requestErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := c.Context().Err(); err != nil {
			return nil, err
		}
		resp, err := c.authorize(c.contentClient.R().SetContext(c.Context())).SetQueryParams(c.commonParams()).SetHeader("Content-Type", "application/json").SetBody(map[string]any{"noteIds": ids}).Post("note/getNoteContents")
		requestErr = err
		if err != nil && len(ids) > 1 {
			break
		}
		if err == nil {
			body, status = resp.Body(), resp.StatusCode()
			if status < 500 {
				break
			}
		}
		if attempt < 3 {
			c.waitRetry(time.Duration(attempt) * 300 * time.Millisecond)
		}
	}
	if err := c.Context().Err(); err != nil {
		return nil, err
	}
	// Reduce a batch on a slow read or a server-enforced byte budget. Completed
	// chunks are never acknowledged by the caller until the page is persisted.
	if (requestErr != nil || status == http.StatusRequestEntityTooLarge) && len(ids) > 1 {
		mid := len(ids) / 2
		left, err := c.GetNoteContents(ids[:mid])
		if err != nil {
			return nil, err
		}
		right, err := c.GetNoteContents(ids[mid:])
		return append(left, right...), err
	}
	if requestErr != nil {
		return nil, c.redactError(requestErr)
	}
	if err := checkSyncListResponse(status, body); err != nil {
		return nil, err
	}
	var notes []*models.Note
	if err := json.Unmarshal(body, &notes); err != nil {
		return nil, err
	}
	expected := map[string]bool{}
	for _, id := range ids {
		if expected[id] {
			return nil, fmt.Errorf("duplicate note ID")
		}
		expected[id] = true
	}
	if len(notes) != len(ids) {
		return nil, fmt.Errorf("incomplete note content batch")
	}
	for _, note := range notes {
		if note == nil || !expected[note.NoteID] || !note.ContentPresent {
			return nil, fmt.Errorf("invalid note content batch")
		}
		delete(expected, note.NoteID)
	}
	return notes, nil
}

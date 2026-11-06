package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/logger"
	"github.com/google/uuid"
)

const LiveListenBrainz = "listenbrainz-live-v1"
const MaxLiveRequest = 8 << 20

// LiveEnvelope preserves transport facts separately from unchanged request
// bytes. Credentials and cookies are deliberately absent from this structure.
type LiveEnvelope struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"received_at"`
	UserID     int32     `json:"user_id"`
	File       File      `json:"file"`
	RequestKey string    `json:"request_key,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	RelayURL   string    `json:"relay_url,omitempty"`
}

type LivePayload struct {
	ListenType string            `json:"listen_type"`
	Payload    []json.RawMessage `json:"payload"`
}

// DecodeLive validates the protocol envelope without rewriting or selecting
// fields from the payload. Completion and media qualification belong to filters.
func DecodeLive(data []byte) (LivePayload, error) {
	var request LivePayload
	if len(data) > MaxLiveRequest {
		return request, errors.New("Submission is too large.")
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return request, errors.New("Invalid submission JSON.")
	}
	if request.ListenType == "" {
		request.ListenType = "single"
	}
	if request.ListenType != "single" && request.ListenType != "import" && request.ListenType != "playing_now" {
		return request, errors.New("Unknown listen type.")
	}
	if len(request.Payload) > 1000 || len(request.Payload) == 0 && request.ListenType != "playing_now" || len(request.Payload) > 1 && request.ListenType != "import" {
		return request, errors.New("Invalid submission payload size.")
	}
	for _, payload := range request.Payload {
		var row struct {
			ListenedAt int64 `json:"listened_at"`
			Track      struct {
				Artist string `json:"artist_name"`
				Title  string `json:"track_name"`
			} `json:"track_metadata"`
		}
		if err := json.Unmarshal(payload, &row); err != nil || strings.TrimSpace(row.Track.Artist) == "" || strings.TrimSpace(row.Track.Title) == "" {
			return request, errors.New("Artist and track names are required.")
		}
		if request.ListenType != "playing_now" && (row.ListenedAt <= 0 || row.ListenedAt > 253402300799) {
			return request, errors.New("Completed listens require a positive listened_at timestamp.")
		}
	}
	return request, nil
}

// CaptureLive acknowledges only after the exact bytes and a replayable journal
// entry have both been synced. It never acquires the database writer or view lock.
func (s *Store) CaptureLive(ctx context.Context, data []byte, userID int32, requestKey, userAgent string, relayURL ...string) (LiveEnvelope, error) {
	if _, err := DecodeLive(data); err != nil {
		return LiveEnvelope{}, err
	}
	if userID <= 0 {
		return LiveEnvelope{}, errors.New("Invalid user.")
	}
	if len(requestKey) > 500 || len(userAgent) > 2000 {
		return LiveEnvelope{}, errors.New("Submission headers are too long.")
	}
	file, err := s.saveContent(ctx, bytes.NewReader(data))
	if err != nil {
		return LiveEnvelope{}, err
	}
	entry := LiveEnvelope{ID: uuid.NewString(), ReceivedAt: time.Now().UTC(), UserID: userID, File: file, RequestKey: requestKey, UserAgent: userAgent}
	if len(relayURL) > 0 {
		entry.RelayURL = relayURL[0]
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return LiveEnvelope{}, err
	}
	directory := filepath.Join(s.Root, "live", "pending")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return LiveEnvelope{}, err
	}
	temporary, err := os.CreateTemp(directory, ".capture-*")
	if err != nil {
		return LiveEnvelope{}, err
	}
	defer temporary.Close()
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(encoded); err != nil {
		return LiveEnvelope{}, err
	}
	if err = temporary.Sync(); err != nil {
		return LiveEnvelope{}, err
	}
	if err = temporary.Close(); err != nil {
		return LiveEnvelope{}, err
	}
	if err = os.Rename(temporary.Name(), filepath.Join(directory, entry.ID+".json")); err != nil {
		return LiveEnvelope{}, err
	}
	return entry, nil
}

func (s *Store) PendingLive(ctx context.Context) ([]LiveEnvelope, error) {
	directory := filepath.Join(s.Root, "live", "pending")
	files, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []LiveEnvelope
	for _, file := range files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, file.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var entry LiveEnvelope
		if err = json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("Read live journal %s: %w", file.Name(), err)
		}
		if _, err = uuid.Parse(entry.ID); err != nil || file.Name() != entry.ID+".json" || entry.UserID <= 0 || entry.ReceivedAt.IsZero() {
			return nil, fmt.Errorf("Invalid live journal %s", file.Name())
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ReceivedAt.Equal(entries[j].ReceivedAt) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].ReceivedAt.Before(entries[j].ReceivedAt)
	})
	return entries, nil
}

// StoreLive is restart-safe. Each delivery is retained. Only an explicit retry
// key reuses records at intake. Other potential duplicates remain filter input.
func (s *Store) StoreLive(ctx context.Context, entry LiveEnvelope) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	path, err := s.FilePath(entry.File.Hash)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if digest(data) != entry.File.Hash || int64(len(data)) != entry.File.Size {
		return errors.New("Live request failed integrity check.")
	}
	request, err := DecodeLive(data)
	if err != nil {
		return err
	}
	if _, err = s.SaveFile(ctx, bytes.NewReader(data)); err != nil {
		return err
	}
	source, err := s.ensureSource(ctx, "listenbrainz", "", entry.File.Hash, entry.UserID)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_submissions WHERE id=?)`, entry.ID).Scan(&exists); err != nil || exists {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO live_submissions(id,source_id,file_hash,listen_type,received_at,request_key,user_agent) VALUES(?,?,?,?,?,?,?)`, entry.ID, source, entry.File.Hash, request.ListenType, entry.ReceivedAt.Format(time.RFC3339Nano), entry.RequestKey, entry.UserAgent)
	if err != nil {
		return err
	}
	// An explicit retry key is only meaningful within this user and request kind.
	// Conflicting bytes remain separate evidence rather than silently disappearing.
	if entry.RequestKey != "" {
		result, err := tx.ExecContext(ctx, `INSERT INTO live_submission_records(submission_id,position,record_id)
		 SELECT ?,r.position,r.record_id FROM live_submission_records r JOIN live_submissions d ON d.id=r.submission_id
		 WHERE d.id=(SELECT id FROM live_submissions WHERE source_id=? AND request_key=? AND file_hash=? AND listen_type=? AND id<>? ORDER BY received_at,id LIMIT 1)`, entry.ID, source, entry.RequestKey, entry.File.Hash, request.ListenType, entry.ID)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil {
			return err
		} else if n > 0 {
			return tx.Commit()
		}
	}
	var ordinal int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ordinal),-1)+1 FROM source_records WHERE source_id=? AND file_hash=? AND parser=?`, source, entry.File.Hash, LiveListenBrainz).Scan(&ordinal); err != nil {
		return err
	}
	for position, raw := range request.Payload {
		kind := "play"
		if request.ListenType == "playing_now" {
			kind = "now_playing"
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO source_records(source_id,file_hash,ordinal,kind,payload,parser) VALUES(?,?,?,?,?,?)`, source, entry.File.Hash, ordinal+int64(position), kind, string(raw), LiveListenBrainz)
		if err != nil {
			return err
		}
		record, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO live_submission_records(submission_id,position,record_id) VALUES(?,?,?)`, entry.ID, position, record); err != nil {
			return err
		}
		if kind == "now_playing" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO source_interpretations(record_id,policy,play_key,reason) VALUES(?,?,'','now playing')`, record, Policy); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// RunLive recovers pending entries at startup and retries failed work. Removal
// follows successful publication, so a stopped process simply replays its journal.
func (s *Store) RunLive(ctx context.Context) {
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		if err := s.ProcessLive(ctx); err != nil && ctx.Err() == nil {
			logger.FromContext(ctx).Error().Err(err).Msg("Live submissions remain queued")
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (s *Store) ProcessLive(ctx context.Context) error {
	entries, err := s.PendingLive(ctx)
	if err != nil || len(entries) == 0 {
		return err
	}
	completed := false
	for _, entry := range entries {
		if err = s.StoreLive(ctx, entry); err != nil {
			return err
		}
		data, err := s.ReadLiveRequest(entry)
		if err != nil {
			return err
		}
		request, err := DecodeLive(data)
		if err != nil {
			return err
		}
		var stored bool
		if err = s.DB.QueryRowContext(ctx, `SELECT status='stored' FROM live_submissions WHERE id=?`, entry.ID).Scan(&stored); err != nil {
			return err
		}
		completed = completed || request.ListenType != "playing_now" && stored
	}
	if completed {
		if _, err = s.Rebuild(ctx); err != nil {
			return err
		}
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE live_submissions SET status='projected',version_id=(SELECT version_id FROM filter_state WHERE id=1) WHERE listen_type='playing_now' AND status='stored'`)
		if err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if entry.RelayURL != "" && !s.liveRelayDone(entry) {
			continue
		}
		if err = os.Remove(filepath.Join(s.Root, "live", "pending", entry.ID+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// ReadLiveRequest is also used by diagnostics and tests to compare original
// bytes with an acknowledged submission.
func (s *Store) ReadLiveRequest(entry LiveEnvelope) ([]byte, error) {
	path, err := s.FilePath(entry.File.Hash)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, MaxLiveRequest+1))
}

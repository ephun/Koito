package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/logger"
)

func (s *Store) liveRelayDone(entry LiveEnvelope) bool {
	data, err := os.ReadFile(filepath.Join(s.Root, "live", "relayed", entry.ID))
	return err == nil && string(data) == entry.File.Hash+"\n"+entry.RelayURL
}

// ForwardLive retries durable relay jobs independently of catalog publication.
// Each retry recreates its request body and keeps the same delivery identifier.
func (s *Store) ForwardLive(ctx context.Context, endpoint, token string) error {
	entries, err := s.PendingLive(ctx)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	for _, entry := range entries {
		if entry.RelayURL == "" || s.liveRelayDone(entry) {
			continue
		}
		if entry.RelayURL != endpoint {
			return errors.New("A queued relay destination differs from the configured destination.")
		}
		data, err := s.ReadLiveRequest(entry)
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/submit-listens", bytes.NewReader(data))
		if err != nil {
			return errors.New("Invalid relay destination.")
		}
		request.Header.Set("Authorization", "Token "+token)
		request.Header.Set("Content-Type", "application/json")
		key := entry.RequestKey
		if key == "" {
			key = entry.ID
		}
		request.Header.Set("Idempotency-Key", key)
		response, err := client.Do(request)
		if err != nil {
			return errors.New("Relay connection failed. The submission remains queued.")
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("Relay returned HTTP %d. The submission remains queued.", response.StatusCode)
		}
		directory := filepath.Join(s.Root, "live", "relayed")
		if err = os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		file, err := os.CreateTemp(directory, ".relay-*")
		if err != nil {
			return err
		}
		name := file.Name()
		_, writeErr := file.WriteString(entry.File.Hash + "\n" + entry.RelayURL)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr == nil {
			writeErr = os.Rename(name, filepath.Join(directory, entry.ID))
		}
		os.Remove(name)
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (s *Store) RunLiveRelay(ctx context.Context, endpoint, token string) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		if err := s.ForwardLive(ctx, endpoint, token); err != nil && ctx.Err() == nil {
			logger.FromContext(ctx).Error().Err(err).Msg("Live relay remains queued")
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

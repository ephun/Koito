package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type ImportResult struct {
	ReceiptID int64  `json:"receipt_id"`
	Hash      string `json:"hash"`
	Records   int    `json:"records"`
	Added     int    `json:"added"`
	Reused    int    `json:"reused"`
}

// ImportSpotify keeps the original API available for older callers and tests.
func (s *Store) ImportSpotify(ctx context.Context, input io.Reader, filename, account string, userID int32) (ImportResult, error) {
	return s.importFile(ctx, input, filename, account, userID, "spotify")
}

func (s *Store) parseSpotify(f File) ([]json.RawMessage, error) {
	p, _ := s.FilePath(f.Hash)
	file, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	token, err := dec.Token()
	if err != nil || token != json.Delim('[') {
		return nil, errors.New("Unsupported file.")
	}
	out := []json.RawMessage{}
	for dec.More() {
		var raw json.RawMessage
		if err = dec.Decode(&raw); err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return nil, errors.New("every history entry must be an object")
		}
		// Reject other JSON exports, but retain missing or invalid values for review.
		if _, ok := fields["ts"]; !ok {
			return nil, errors.New("Unsupported file.")
		}
		if _, ok := fields["ms_played"]; !ok {
			return nil, errors.New("Unsupported file.")
		}
		out = append(out, bytes.Clone(raw))
	}
	if _, err = dec.Token(); err != nil {
		return nil, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("unexpected content after history array")
	}
	return out, nil
}

func spotifyKind(raw json.RawMessage) string {
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	if title, ok := v["master_metadata_track_name"].(string); ok && title != "" {
		return "play"
	}
	if v["episode_name"] != nil {
		return "podcast"
	}
	if v["audiobook_title"] != nil {
		return "audiobook"
	}
	return "unknown"
}

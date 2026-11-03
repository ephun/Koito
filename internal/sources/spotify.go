package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ImportResult struct {
	ReceiptID int64  `json:"receipt_id"`
	Hash      string `json:"hash"`
	Records   int    `json:"records"`
	Added     int    `json:"added"`
	Reused    int    `json:"reused"`
}
type parsedFile struct {
	file    File
	records []json.RawMessage
}

// ImportSpotify preserves both the container and member bytes. Record extraction
// commits once, after every history file validates, preventing partial imports.
func (s *Store) ImportSpotify(ctx context.Context, input io.Reader, filename, account string, userID int32) (result ImportResult, err error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	account = strings.TrimSpace(account)
	if account == "" || len(account) > 200 {
		return result, errors.New("an account name of 1–200 characters is required")
	}
	f, err := s.SaveFile(ctx, input)
	if err != nil {
		return result, err
	}
	result.Hash = f.Hash
	_, err = s.DB.ExecContext(ctx, `INSERT INTO source_accounts(service,account,user_id) VALUES('spotify',?,?) ON CONFLICT(service,account) DO NOTHING`, account, userID)
	if err != nil {
		return result, err
	}
	var source int64
	var owner int32
	err = s.DB.QueryRowContext(ctx, `SELECT id,user_id FROM source_accounts WHERE service='spotify' AND account=?`, account).Scan(&source, &owner)
	if err != nil {
		return result, err
	}
	if owner != userID {
		return result, errors.New("account is assigned to a different user")
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO import_receipts(source_id,file_hash,filename,status,created_at) VALUES(?,?,?,'processing',?)`, source, f.Hash, filepath.Base(filename), time.Now().Unix())
	if err != nil {
		return result, err
	}
	result.ReceiptID, err = res.LastInsertId()
	if err != nil {
		return result, err
	}
	defer func() {
		status, message := "stored", ""
		if err != nil {
			status = "failed"
			message = err.Error()
		}
		// Persist terminal status even if the request was cancelled.
		finish, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, e := s.DB.ExecContext(finish, `UPDATE import_receipts SET status=?,error=? WHERE id=?`, status, message, result.ReceiptID)
		if err == nil {
			err = e
		}
	}()
	var parsed []parsedFile
	p, _ := s.FilePath(f.Hash)
	if strings.EqualFold(filepath.Ext(filename), ".zip") {
		z, e := zip.OpenReader(p)
		if e != nil {
			return result, fmt.Errorf("invalid ZIP: %w", e)
		}
		defer z.Close()
		if len(z.File) > 10000 {
			return result, errors.New("ZIP has too many entries")
		}
		var expanded uint64
		for pos, member := range z.File {
			if member.FileInfo().IsDir() {
				continue
			}
			if member.UncompressedSize64 > uint64(MaxFileSize)-expanded {
				return result, errors.New("ZIP expanded size exceeds limit")
			}
			expanded += member.UncompressedSize64
			name := filepath.Base(strings.ReplaceAll(member.Name, "\\", "/"))
			if !strings.HasPrefix(name, "Streaming_History_") || !strings.EqualFold(filepath.Ext(name), ".json") {
				continue
			}
			stream, e := member.Open()
			if e != nil {
				return result, e
			}
			child, e := s.SaveFile(ctx, stream)
			stream.Close()
			if e != nil {
				return result, e
			}
			_, e = s.DB.ExecContext(ctx, `INSERT INTO source_members(parent_hash,position,name,file_hash) VALUES(?,?,?,?) ON CONFLICT(parent_hash,position) DO NOTHING`, f.Hash, pos, member.Name, child.Hash)
			if e != nil {
				return result, e
			}
			rows, e := s.parseSpotify(child)
			if e != nil {
				return result, fmt.Errorf("%s: %w", member.Name, e)
			}
			parsed = append(parsed, parsedFile{child, rows})
		}
		if len(parsed) == 0 {
			return result, errors.New("ZIP contains no Spotify extended Streaming_History_*.json files")
		}
	} else {
		rows, e := s.parseSpotify(f)
		if e != nil {
			return result, e
		}
		parsed = append(parsed, parsedFile{f, rows})
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	for _, part := range parsed {
		result.Records += len(part.records)
		var count int
		e := tx.QueryRowContext(ctx, `SELECT record_count FROM source_extractions WHERE source_id=? AND file_hash=? AND parser=?`, source, part.file.Hash, Parser).Scan(&count)
		if e == nil {
			result.Reused += count
			continue
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return result, e
		}
		stmt, e := tx.PrepareContext(ctx, `INSERT INTO source_records(source_id,file_hash,ordinal,kind,payload,parser) VALUES(?,?,?,?,?,?)`)
		if e != nil {
			return result, e
		}
		for i, raw := range part.records {
			_, e = stmt.ExecContext(ctx, source, part.file.Hash, i, spotifyKind(raw), string(raw), Parser)
			if e != nil {
				stmt.Close()
				return result, e
			}
			result.Added++
		}
		stmt.Close()
		_, e = tx.ExecContext(ctx, `INSERT INTO source_extractions(source_id,file_hash,parser,record_count) VALUES(?,?,?,?)`, source, part.file.Hash, Parser, len(part.records))
		if e != nil {
			return result, e
		}
	}
	err = tx.Commit()
	return result, err
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
		return nil, errors.New("expected a Spotify extended history JSON array")
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
			return nil, errors.New("entry lacks extended-history ts field")
		}
		if _, ok := fields["ms_played"]; !ok {
			return nil, errors.New("entry lacks extended-history ms_played field")
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

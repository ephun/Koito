package sources

import (
	"archive/zip"
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

func (s *Store) Import(ctx context.Context, input io.Reader, filename string, userID int32) (ImportResult, error) {
	return s.importFile(ctx, input, filename, "", userID, "")
}

func (s *Store) ensureSource(ctx context.Context, service, account, hash string, userID int32) (int64, error) {
	account = strings.TrimSpace(account)
	if len(account) > 200 {
		return 0, errors.New("Source label is too long.")
	}
	if account == "" {
		err := s.DB.QueryRowContext(ctx, `SELECT a.account FROM source_accounts a WHERE a.service=? AND a.user_id=? AND (
		 EXISTS(SELECT 1 FROM source_extractions e WHERE e.source_id=a.id AND e.file_hash=?) OR
		 EXISTS(SELECT 1 FROM import_receipts r WHERE r.source_id=a.id AND r.file_hash=? AND r.status='stored')
		) ORDER BY a.id LIMIT 1`, service, userID, hash, hash).Scan(&account)
		if errors.Is(err, sql.ErrNoRows) {
			account = fmt.Sprintf("user:%d", userID)
		} else if err != nil {
			return 0, err
		}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO source_accounts(service,account,user_id) VALUES(?,?,?) ON CONFLICT(service,account) DO NOTHING`, service, account, userID)
	if err != nil {
		return 0, err
	}
	var id int64
	var owner int32
	err = s.DB.QueryRowContext(ctx, `SELECT id,user_id FROM source_accounts WHERE service=? AND account=?`, service, account).Scan(&id, &owner)
	if err == nil && owner != userID {
		err = errors.New("Source belongs to another user.")
	}
	return id, err
}

func (s *Store) importFile(ctx context.Context, input io.Reader, filename, account string, userID int32, expected string) (result ImportResult, err error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	f, err := s.SaveFile(ctx, input)
	if err != nil {
		return result, err
	}
	result.Hash = f.Hash
	parts, parseErr := s.readUpload(ctx, f, filename)
	service := expected
	if service == "" {
		service = "unknown"
		if len(parts) > 0 {
			service = parts[0].service
			for _, part := range parts {
				if part.service != service {
					service = "archive"
					break
				}
			}
		}
	}
	source, err := s.ensureSource(ctx, service, account, f.Hash, userID)
	if err != nil {
		return result, err
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
		finish, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, e := s.DB.ExecContext(finish, `UPDATE import_receipts SET status=?,error=? WHERE id=?`, status, message, result.ReceiptID)
		if err == nil {
			err = e
		}
	}()
	if parseErr != nil {
		return result, parseErr
	}
	sources := make([]int64, len(parts))
	for i, part := range parts {
		if expected != "" && part.service != expected {
			return result, ErrUnsupportedFile
		}
		sources[i], err = s.ensureSource(ctx, part.service, account, part.file.Hash, userID)
		if err != nil {
			return result, err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	for i, part := range parts {
		result.Records += len(part.records)
		var count int
		e := tx.QueryRowContext(ctx, `SELECT record_count FROM source_extractions WHERE source_id=? AND file_hash=? AND parser=?`, sources[i], part.file.Hash, part.parser).Scan(&count)
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
		kinds := map[int]string{}
		for _, snapshot := range part.snapshots {
			for _, ordinal := range snapshot.Ordinals {
				kinds[ordinal] = snapshot.Kind
			}
		}
		for ordinal, raw := range part.records {
			kind := "play"
			if part.service == "itunes" {
				var row itunesRecord
				if e = json.Unmarshal(raw, &row); e != nil {
					stmt.Close()
					return result, e
				}
				kind = row.Kind
			} else if snapshotParser(part.parser) {
				kind = "context"
				kind = kinds[ordinal]
			} else if part.service == "spotify" {
				kind = spotifyKind(raw)
			} else if normalizeRecord(raw, part.parser, "").NowPlaying {
				kind = "now_playing"
			}
			_, e = stmt.ExecContext(ctx, sources[i], part.file.Hash, ordinal, kind, string(raw), part.parser)
			if e != nil {
				stmt.Close()
				return result, e
			}
			result.Added++
		}
		stmt.Close()
		if e = storeSnapshots(ctx, tx, sources[i], part); e != nil {
			return result, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO source_extractions(source_id,file_hash,parser,record_count) VALUES(?,?,?,?)`, sources[i], part.file.Hash, part.parser, len(part.records))
		if e != nil {
			return result, e
		}
	}
	err = tx.Commit()
	return result, err
}

func (s *Store) readUpload(ctx context.Context, f File, filenames ...string) ([]parsedFile, error) {
	p, _ := s.FilePath(f.Hash)
	probe, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	var signature [4]byte
	_, err = io.ReadFull(probe, signature[:])
	probe.Close()
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	if string(signature[:]) != "PK\x03\x04" && string(signature[:]) != "PK\x05\x06" {
		part, e := s.parseFile(f, filenames...)
		if e != nil {
			return nil, e
		}
		return []parsedFile{part}, nil
	}
	z, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("Invalid ZIP: %w", err)
	}
	defer z.Close()
	if len(z.File) > 10000 {
		return nil, errors.New("ZIP has too many entries.")
	}
	var total uint64
	var parts []parsedFile
	for pos, member := range z.File {
		if member.FileInfo().IsDir() {
			continue
		}
		if member.UncompressedSize64 > uint64(MaxFileSize)-total {
			return nil, errors.New("ZIP expanded size exceeds limit")
		}
		total += member.UncompressedSize64
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		stream, e := member.Open()
		if e != nil {
			return nil, e
		}
		child, e := s.SaveFile(ctx, stream)
		stream.Close()
		if e != nil {
			return nil, e
		}
		_, e = s.DB.ExecContext(ctx, `INSERT INTO source_members(parent_hash,position,name,file_hash) VALUES(?,?,?,?) ON CONFLICT(parent_hash,position) DO NOTHING`, f.Hash, pos, member.Name, child.Hash)
		if e != nil {
			return nil, e
		}
		part, e := s.parseFile(child, member.Name)
		if errors.Is(e, ErrUnsupportedFile) && !strings.HasPrefix(filepath.Base(strings.ReplaceAll(member.Name, "\\", "/")), "Streaming_History_") {
			continue
		}
		if e != nil {
			return nil, fmt.Errorf("%s: %w", member.Name, e)
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, ErrUnsupportedFile
	}
	return parts, nil
}

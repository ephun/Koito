// Package sources preserves original files and records independently of the
// disposable catalog projection. It does not apply qualification during import.
package sources

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxFileSize int64 = 512 << 20
const Parser = "spotify-extended-v1"
const Policy = "spotify-v1"

// A process-wide lock serializes import publication and projection changes even
// when handlers construct different Store values for the same database.
var writeMu sync.Mutex

type Store struct {
	DB   *sql.DB
	Root string
}

type File struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}
type Receipt struct {
	ID        int64  `json:"id"`
	SourceID  int64  `json:"source_id"`
	Account   string `json:"account"`
	Hash      string `json:"hash"`
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	CreatedAt int64  `json:"created_at"`
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func (s *Store) FilePath(hash string) (string, error) {
	b, err := hex.DecodeString(hash)
	if err != nil || len(b) != 32 || hex.EncodeToString(b) != hash {
		return "", errors.New("invalid file hash")
	}
	return filepath.Join(s.Root, "sha256", hash[:2], hash), nil
}

// SaveFile writes and syncs a temporary file, then publishes its hash-derived
// name. Existing bytes are verified; never silently replace a damaged archive.
func (s *Store) SaveFile(ctx context.Context, input io.Reader) (File, error) {
	var result File
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return result, err
	}
	f, err := os.CreateTemp(s.Root, ".upload-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(input, MaxFileSize+1))
	if err != nil {
		return result, err
	}
	if n > MaxFileSize {
		return result, fmt.Errorf("file exceeds %d bytes", MaxFileSize)
	}
	result = File{hex.EncodeToString(h.Sum(nil)), n}
	if err = f.Sync(); err != nil {
		return result, err
	}
	if err = f.Close(); err != nil {
		return result, err
	}
	dest, _ := s.FilePath(result.Hash)
	if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return result, err
	}
	// Link is an atomic no-replace publication on supported filesystems, including NTFS.
	if err = os.Link(f.Name(), dest); err != nil {
		existing, e := os.Open(dest)
		if e != nil {
			return result, err
		}
		check := sha256.New()
		count, e := io.Copy(check, existing)
		existing.Close()
		if e != nil || count != n || hex.EncodeToString(check.Sum(nil)) != result.Hash {
			return result, errors.New("stored file failed integrity check")
		}
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO source_files(hash,size,created_at) VALUES(?,?,?) ON CONFLICT(hash) DO NOTHING`, result.Hash, n, time.Now().Unix())
	return result, err
}

func (s *Store) Receipts(ctx context.Context) ([]Receipt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.source_id,a.account,r.file_hash,r.filename,r.status,r.error,r.created_at FROM import_receipts r JOIN source_accounts a ON a.id=r.source_id ORDER BY r.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Receipt{}
	for rows.Next() {
		var v Receipt
		if err = rows.Scan(&v.ID, &v.SourceID, &v.Account, &v.Hash, &v.Filename, &v.Status, &v.Error, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Record struct {
	ID       int64  `json:"id"`
	FileHash string `json:"file_hash"`
	Ordinal  int    `json:"ordinal"`
	Kind     string `json:"kind"`
	Payload  string `json:"payload"`
	Reason   string `json:"reason"`
	PlayKey  string `json:"play_key"`
}

func (s *Store) Records(ctx context.Context, source int64, after int64) ([]Record, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.file_hash,r.ordinal,r.kind,r.payload,COALESCE(i.reason,'not projected'),COALESCE(i.play_key,'') FROM source_records r LEFT JOIN source_interpretations i ON i.record_id=r.id AND i.policy=? WHERE r.source_id=? AND r.id>? ORDER BY r.id LIMIT 100`, Policy, source, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var v Record
		if err = rows.Scan(&v.ID, &v.FileHash, &v.Ordinal, &v.Kind, &v.Payload, &v.Reason, &v.PlayKey); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

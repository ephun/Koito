package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Library counts describe evidence, before attribution or inference. All
// contributing records remain available, including equivalent converted files.
type LibrarySnapshot struct {
	RecordID   int64    `json:"record_id"`
	FileHash   string   `json:"file_hash"`
	Snapshot   string   `json:"snapshot,omitempty"`
	Count      *int64   `json:"count"`
	CountState string   `json:"count_state"`
	Added      string   `json:"added,omitempty"`
	LastPlayed string   `json:"last_played,omitempty"`
	Issues     []string `json:"issues"`
}

type LibraryTrack struct {
	Key          string            `json:"key"`
	PersistentID string            `json:"persistent_id,omitempty"`
	Title        string            `json:"title"`
	Artist       string            `json:"artist"`
	Album        string            `json:"album"`
	Music        bool              `json:"music"`
	Reported     *int64            `json:"reported"`
	Latest       *int64            `json:"latest"`
	Issues       []string          `json:"issues"`
	Snapshots    []LibrarySnapshot `json:"snapshots"`
}

type LibraryFile struct {
	Hash        string   `json:"hash"`
	SourceID    int64    `json:"source_id"`
	LibraryID   string   `json:"library_id,omitempty"`
	Snapshot    string   `json:"snapshot,omitempty"`
	Fingerprint string   `json:"fingerprint"`
	Converted   bool     `json:"converted"`
	Equivalent  bool     `json:"equivalent"`
	Issues      []string `json:"issues"`
}

type LibraryEvidence struct {
	Key          string         `json:"key"`
	LibraryID    string         `json:"library_id,omitempty"`
	Reported     int64          `json:"reported"`
	Unknown      int            `json:"unknown_counts"`
	Invalid      int            `json:"invalid_counts"`
	Unresolved   int            `json:"unresolved_tracks"`
	Unidentified int            `json:"unidentified_tracks"`
	Files        []LibraryFile  `json:"files"`
	Tracks       []LibraryTrack `json:"tracks"`
}

type libraryTrackInput struct {
	track    LibraryTrack
	snapshot LibrarySnapshot
}

type libraryFileInput struct {
	file   LibraryFile
	userID int32
	tracks []libraryTrackInput
	hashes []string
}

type libraryEnvelope struct {
	Kind      string                     `json:"kind"`
	LibraryID string                     `json:"library_id"`
	Snapshot  string                     `json:"snapshot"`
	Fields    map[string]json.RawMessage `json:"fields"`
}

func libraryText(fields map[string]json.RawMessage, name string) string {
	return rawString(itunesScalar(fields[name]))
}

func libraryCount(fields map[string]json.RawMessage) (*int64, string) {
	raw, exists := fields["Play Count"]
	if !exists || string(raw) == "null" {
		return nil, "unknown"
	}
	text := rawString(itunesScalar(raw))
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < 0 {
		return nil, "invalid"
	}
	return &value, "reported"
}

func libraryDate(text string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil || t.Year() < 1970 {
		return time.Time{}
	}
	return t.UTC()
}

// The historical R conversion converted XML scalars into strings, wrapped
// scalars in arrays, and kept the last repeated field. This fingerprint only
// recognizes equivalent copies. It does not replace preserved source values.
func libraryCopyFingerprint(fields map[string]json.RawMessage) string {
	values := map[string]any{}
	for k, raw := range fields {
		var repeated struct {
			Values []json.RawMessage `json:"$plist_repeated"`
		}
		if json.Unmarshal(raw, &repeated) == nil && len(repeated.Values) > 0 {
			raw = repeated.Values[len(repeated.Values)-1]
		}
		raw = itunesScalar(raw)
		if string(raw) == "true" || string(raw) == "false" {
			values[k] = ""
		} else if text := rawString(raw); text != "" || string(raw) == `""` {
			values[k] = text
		} else {
			// Complex values retain their structure. The conversion's scalar
			// assumptions must not collapse unrelated arrays or objects.
			values[k] = fields[k]
		}
	}
	canonical, _ := json.Marshal(values)
	return digest(canonical)
}

func readLibraryTrack(id int64, file LibraryFile, row libraryEnvelope) libraryTrackInput {
	f := row.Fields
	count, state := libraryCount(f)
	snapshot := LibrarySnapshot{RecordID: id, FileHash: file.Hash, Snapshot: row.Snapshot, Count: count, CountState: state, Added: libraryText(f, "Date Added"), LastPlayed: libraryText(f, "Play Date UTC"), Issues: []string{}}
	added, last, date := libraryDate(snapshot.Added), libraryDate(snapshot.LastPlayed), libraryDate(row.Snapshot)
	if snapshot.Added != "" && added.IsZero() {
		snapshot.Issues = append(snapshot.Issues, "invalid date added")
	}
	if snapshot.LastPlayed != "" && last.IsZero() {
		snapshot.Issues = append(snapshot.Issues, "invalid last played")
	}
	if !added.IsZero() && !last.IsZero() && last.Before(added) {
		snapshot.Issues = append(snapshot.Issues, "last played precedes date added")
	}
	if !date.IsZero() && ((!last.IsZero() && last.After(date)) || (!added.IsZero() && added.After(date))) {
		snapshot.Issues = append(snapshot.Issues, "track date exceeds snapshot")
	}
	if count != nil && *count == 0 && !last.IsZero() {
		snapshot.Issues = append(snapshot.Issues, "last played with zero count")
	}
	if state == "invalid" {
		snapshot.Issues = append(snapshot.Issues, "invalid play count")
	}
	persistent := libraryText(f, "Persistent ID")
	key := "persistent:" + persistent
	issues := []string{}
	if strings.TrimSpace(persistent) == "" {
		key = fmt.Sprintf("record:%d", id)
		issues = append(issues, "unknown persistent identity")
	}
	nonmusic := false
	for _, flag := range []string{"Podcast", "Has Video", "Movie", "TV Show", "Audiobook"} {
		raw := itunesScalar(f[flag])
		// The R conversion loses boolean values but preserves the field.
		if value, present := f[flag]; present && string(value) != "null" && (string(raw) == "true" || string(raw) == `""` || libraryText(f, flag) == "true") {
			nonmusic = true
		}
	}
	if kind := strings.ToLower(libraryText(f, "Kind")); strings.Contains(kind, "video") || strings.Contains(kind, "audiobook") {
		nonmusic = true
	}
	track := LibraryTrack{Key: key, PersistentID: persistent, Title: libraryText(f, "Name"), Artist: libraryText(f, "Artist"), Album: libraryText(f, "Album"), Music: !nonmusic, Issues: issues}
	return libraryTrackInput{track: track, snapshot: snapshot}
}

func loadLibraryFiles(ctx context.Context, reader sqlReader) ([]*libraryFileInput, error) {
	rows, err := reader.QueryContext(ctx, `SELECT r.id,r.file_hash,r.source_id,a.user_id,r.parser,r.payload FROM source_records r JOIN source_accounts a ON a.id=r.source_id WHERE a.service IN ('itunes','navidrome') AND r.kind IN ('library','library_track') ORDER BY r.source_id,r.file_hash,r.ordinal`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []*libraryFileInput{}
	byFile := map[string]*libraryFileInput{}
	for rows.Next() {
		var id, source int64
		var user int32
		var hash, parser, payload string
		if err := rows.Scan(&id, &hash, &source, &user, &parser, &payload); err != nil {
			return nil, err
		}
		var envelope libraryEnvelope
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			return nil, err
		}
		if databaseFormat(parser) == NavidromeDB {
			var record databaseRecord
			if err := json.Unmarshal([]byte(payload), &record); err != nil {
				return nil, err
			}
			envelope = navidromeLibraryRecord(record)
		}
		key := fmt.Sprintf("%d:%s:%s:%s", source, hash, parser, envelope.LibraryID)
		current := byFile[key]
		if current == nil {
			current = &libraryFileInput{file: LibraryFile{Hash: hash, SourceID: source, Converted: parser == ITunesJSON, Issues: []string{}}, userID: user}
			byFile[key] = current
			files = append(files, current)
		}
		current.file.LibraryID = envelope.LibraryID
		current.file.Snapshot = envelope.Snapshot
		if envelope.Kind == "library_track" {
			current.tracks = append(current.tracks, readLibraryTrack(id, current.file, envelope))
			current.hashes = append(current.hashes, libraryCopyFingerprint(envelope.Fields))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, f := range files {
		sort.Strings(f.hashes)
		raw, _ := json.Marshal(f.hashes)
		f.file.Fingerprint = digest(raw)
		f.hashes = nil
		if f.file.Snapshot != "" && libraryDate(f.file.Snapshot).IsZero() {
			f.file.Issues = append(f.file.Issues, "invalid snapshot date")
		}
	}
	return files, nil
}

func summarizeLibraryTrack(track *LibraryTrack) {
	sort.Slice(track.Snapshots, func(i, j int) bool {
		a, b := libraryDate(track.Snapshots[i].Snapshot), libraryDate(track.Snapshots[j].Snapshot)
		if !a.Equal(b) {
			return a.Before(b)
		}
		return track.Snapshots[i].RecordID < track.Snapshots[j].RecordID
	})
	issues := map[string]bool{}
	for _, s := range track.Issues {
		issues[s] = true
	}
	var lastDate time.Time
	var lastCount *int64
	latestConflict := false
	knownCounts := map[int64]bool{}
	undatedCounts := map[int64]bool{}
	var newestSnapshot time.Time
	for _, snapshot := range track.Snapshots {
		if date := libraryDate(snapshot.Snapshot); date.After(newestSnapshot) {
			newestSnapshot = date
		}
	}
	unknownLatest := false
	for _, snapshot := range track.Snapshots {
		for _, s := range snapshot.Issues {
			issues[s] = true
		}
		if snapshot.Count == nil {
			if !newestSnapshot.IsZero() && libraryDate(snapshot.Snapshot).Equal(newestSnapshot) {
				unknownLatest = true
			}
			continue
		}
		value := *snapshot.Count
		if track.Reported == nil || value > *track.Reported {
			copy := value
			track.Reported = &copy
		}
		date := libraryDate(snapshot.Snapshot)
		if date.IsZero() {
			undatedCounts[value] = true
			continue
		}
		knownCounts[value] = true
		if lastCount != nil {
			if date.Equal(lastDate) && value != *lastCount {
				issues["conflicting counts at one snapshot"] = true
				latestConflict = true
				if value < *lastCount {
					continue
				}
			}
			if date.After(lastDate) && value < *lastCount {
				issues["count decreased"] = true
			}
		}
		if date.After(lastDate) {
			latestConflict = false
		}
		copy := value
		lastCount = &copy
		lastDate = date
	}
	track.Latest = lastCount
	if latestConflict || unknownLatest {
		track.Latest = nil
	}
	if unknownLatest {
		issues["latest snapshot has unknown count"] = true
	}
	if lastCount == nil {
		// Undated equivalent copies can corroborate counts but cannot establish
		// which of two different count values came later.
		for _, snapshot := range track.Snapshots {
			if snapshot.Count != nil && track.Reported != nil && *snapshot.Count != *track.Reported {
				issues["unknown snapshot order"] = true
			}
		}
	}
	if lastCount != nil {
		for value := range undatedCounts {
			if !knownCounts[value] {
				issues["unknown snapshot order"] = true
			}
		}
	}
	track.Issues = []string{}
	for s := range issues {
		track.Issues = append(track.Issues, s)
	}
	sort.Strings(track.Issues)
}

func libraryEvidence(ctx context.Context, reader sqlReader) ([]LibraryEvidence, error) {
	files, err := loadLibraryFiles(ctx, reader)
	if err != nil {
		return nil, err
	}
	// A converted copy may inherit library identity only when all its track
	// fields corroborate exactly one known library for the same user.
	known := map[string]map[string]bool{}
	for _, f := range files {
		if f.file.LibraryID == "" {
			continue
		}
		key := fmt.Sprintf("%d:%s", f.userID, f.file.Fingerprint)
		if known[key] == nil {
			known[key] = map[string]bool{}
		}
		known[key][f.file.LibraryID] = true
	}
	groups := map[string]*LibraryEvidence{}
	tracks := map[string]map[string]*LibraryTrack{}
	for _, f := range files {
		library := f.file.LibraryID
		if library == "" && len(f.tracks) > 0 {
			matches := known[fmt.Sprintf("%d:%s", f.userID, f.file.Fingerprint)]
			if len(matches) == 1 {
				for id := range matches {
					library = id
				}
				f.file.Equivalent = true
			} else if len(matches) > 1 {
				f.file.Issues = append(f.file.Issues, "ambiguous library identity")
			}
		}
		identity := library
		if identity == "" {
			identity = "unknown:" + f.file.Fingerprint
			f.file.Issues = append(f.file.Issues, "unknown library identity")
		}
		key := fmt.Sprintf("%d:%s", f.userID, identity)
		if groups[key] == nil {
			groups[key] = &LibraryEvidence{Key: key, LibraryID: library, Files: []LibraryFile{}, Tracks: []LibraryTrack{}}
			tracks[key] = map[string]*LibraryTrack{}
		}
		g := groups[key]
		g.Files = append(g.Files, f.file)
		seen := map[string]bool{}
		for _, entry := range f.tracks {
			track := tracks[key][entry.track.Key]
			if track == nil {
				value := entry.track
				track = &value
				tracks[key][entry.track.Key] = track
			}
			if seen[track.Key] {
				track.Issues = append(track.Issues, "repeated persistent identity")
			}
			seen[track.Key] = true
			if track.Artist != entry.track.Artist || track.Title != entry.track.Title || track.Album != entry.track.Album {
				track.Issues = append(track.Issues, "metadata differs across snapshots")
			}
			if !entry.track.Music {
				track.Music = false
			}
			track.Snapshots = append(track.Snapshots, entry.snapshot)
		}
	}
	out := []LibraryEvidence{}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := groups[key]
		trackKeys := make([]string, 0, len(tracks[key]))
		for k := range tracks[key] {
			trackKeys = append(trackKeys, k)
		}
		sort.Strings(trackKeys)
		for _, k := range trackKeys {
			track := tracks[key][k]
			summarizeLibraryTrack(track)
			if track.PersistentID == "" {
				g.Unidentified++
			}
			if track.Reported != nil && track.PersistentID != "" {
				if *track.Reported > math.MaxInt64-g.Reported {
					return nil, fmt.Errorf("Library counts exceed supported total.")
				}
				g.Reported += *track.Reported
			} else if track.Reported == nil {
				invalid := false
				for _, s := range track.Snapshots {
					if s.CountState == "invalid" {
						invalid = true
					}
				}
				if invalid {
					g.Invalid++
				} else {
					g.Unknown++
				}
			}
			if len(track.Issues) > 0 {
				g.Unresolved++
			}
			g.Tracks = append(g.Tracks, *track)
		}
		out = append(out, *g)
	}
	return out, nil
}

func (s *Store) Libraries(ctx context.Context) ([]LibraryEvidence, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := libraryEvidence(ctx, tx)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

package sources

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

const SpotifyPlaylist = "spotify-playlists-json-v1"
const SpotifyLibrary = "spotify-library-json-v1"
const SpotifyCSV = "spotify-library-csv-v1"
const SpotifyContext = "spotify-account-context-v1"
const DiscogsCSV = "discogs-csv-v1"

func sourceFormat(parser string) string {
	if family := databaseFormat(parser); family != "" {
		return family
	}
	if strings.HasPrefix(parser, SpotifyCSV) {
		return SpotifyCSV
	}
	return parser
}

type parsedSnapshot struct {
	Group    string
	Key      string
	Kind     string
	Name     string
	Metadata json.RawMessage
	Ordinals []int
}

func snapshotParser(parser string) bool {
	return parser == SpotifyPlaylist || parser == SpotifyLibrary || parser == SpotifyContext || strings.HasPrefix(parser, SpotifyCSV) || parser == DiscogsCSV
}

func baseFilename(name string) string { return filepath.Base(strings.ReplaceAll(name, "\\", "/")) }

func parseSnapshots(f File, data []byte, filename string) (parsedFile, error) {
	part := parsedFile{file: f}
	name := baseFilename(filename)
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) == nil && root != nil {
		if playlists, ok := root["playlists"]; ok {
			var rows []map[string]json.RawMessage
			if err := json.Unmarshal(playlists, &rows); err != nil {
				return part, err
			}
			part.service, part.parser = "spotify", SpotifyPlaylist
			group := "spotify-playlists-json:" + name
			part.addSnapshot("manifest:"+group, "snapshot_manifest", name, map[string]any{"filename": name}, nil)
			occurrences := map[string]int{}
			for _, row := range rows {
				var items []json.RawMessage
				if err := json.Unmarshal(row["items"], &items); err != nil {
					return part, err
				}
				label := rawString(row["name"])
				key := rawString(row["uri"])
				if key == "" {
					occurrences[label]++
					key = fmt.Sprintf("file:%s:name:%s:%d", name, label, occurrences[label])
				}
				metadata := map[string]json.RawMessage{}
				for k, v := range row {
					if k != "items" {
						metadata[k] = v
					}
				}
				// A name is identity evidence when the export omits its URI.
				metadata["identity_basis"], _ = json.Marshal("name and occurrence within this export")
				if rawString(row["uri"]) != "" {
					metadata["identity_basis"], _ = json.Marshal("supplied URI")
				}
				part.addSnapshot("playlist:"+key, "playlist", label, metadata, items)
			}
			for i := range part.snapshots {
				part.snapshots[i].Group = group
			}
			return part, nil
		}
		_, tracks := root["tracks"]
		_, albums := root["albums"]
		_, artists := root["artists"]
		if tracks && albums && artists {
			part.service, part.parser = "spotify", SpotifyLibrary
			keys := []string{}
			for key := range root {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				var items []json.RawMessage
				if err := json.Unmarshal(root[key], &items); err != nil {
					items = []json.RawMessage{root[key]}
				}
				kind := map[string]string{"tracks": "saved_tracks", "albums": "saved_albums", "artists": "saved_artists", "shows": "saved_shows", "episodes": "saved_episodes"}[key]
				if kind == "" {
					kind = "context"
				}
				part.addSnapshot("library:"+key, kind, key, map[string]any{"section": key}, items)
			}
			return part, nil
		}
		if spotifyContextPath(filename) || spotifyContextFilename(name) {
			part.service, part.parser = "spotify", SpotifyContext
			part.addSnapshot("context:"+name, "context", name, map[string]any{"filename": name}, []json.RawMessage{bytes.Clone(data)})
			return part, nil
		}
	}
	if json.Valid(data) && (spotifyContextPath(filename) || spotifyContextFilename(name)) && !strings.HasPrefix(name, "StreamingHistory_") && !strings.HasPrefix(name, "Streaming_History_") {
		part.service, part.parser = "spotify", SpotifyContext
		part.addSnapshot("context:"+name, "context", name, map[string]any{"filename": name}, []json.RawMessage{bytes.Clone(data)})
		return part, nil
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	columns, err := reader.Read()
	if err != nil {
		return part, ErrUnsupportedFile
	}
	has := map[string]bool{}
	for _, column := range columns {
		has[column] = true
	}
	kind, label := "", ""
	if has["Track URI"] && has["Track Name"] && has["Artist Name(s)"] {
		part.service, part.parser = "spotify", SpotifyCSV
		kind = "playlist"
		label = strings.TrimSuffix(name, filepath.Ext(name))
		if strings.EqualFold(name, "liked.csv") {
			kind = "saved_tracks"
			label = "tracks"
		}
	} else if has["Album URI"] && has["Album Name"] && has["Album Artist Name(s)"] {
		part.service, part.parser = "spotify", SpotifyCSV
		kind = "saved_albums"
		label = "albums"
	} else if has["release_id"] && (has["Catalog#"] && has["Artist"] && has["Title"] || has["listing_id"] || has["item_id"]) || has["order_num"] && has["order_date"] {
		part.service, part.parser = "discogs", DiscogsCSV
		switch {
		case has["item_id"]:
			kind = "order_items"
		case has["listing_id"]:
			kind = "inventory"
		case has["order_num"]:
			kind = "orders"
		case has["CollectionFolder"]:
			kind = "owned"
		case has["Notes"]:
			kind = "wanted"
		default:
			kind = "contributions"
		}
		label = kind
	} else {
		return part, ErrUnsupportedFile
	}
	items := []json.RawMessage{}
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return part, e
		}
		raw, e := json.Marshal(csvRecord{Columns: columns, Values: row})
		if e != nil {
			return part, e
		}
		items = append(items, raw)
	}
	key := kind
	if kind == "playlist" {
		key = "playlist:file:" + name
		part.parser += ":" + digest([]byte(key))
	}
	if kind == "saved_tracks" {
		key = "library:tracks"
	}
	if kind == "saved_albums" {
		key = "library:albums"
	}
	part.addSnapshot(key, kind, label, map[string]any{"columns": columns, "filename": name, "identity_basis": "filename"}, items)
	return part, nil
}

//go:embed spotify_context_files.txt
var spotifyContextFiles string

var spotifyContextNames = func() map[string]bool {
	names := map[string]bool{}
	for _, name := range strings.Split(spotifyContextFiles, "\n") {
		names[strings.TrimSpace(name)] = true
	}
	return names
}()

func spotifyContextFilename(name string) bool {
	return name != "" && spotifyContextNames[name]
}

func spotifyContextPath(name string) bool {
	path := strings.ReplaceAll(name, "\\", "/")
	return strings.Contains(path, "Spotify Account Data/") || strings.Contains(path, "Spotify Technical Log Information/")
}

func (part *parsedFile) addSnapshot(key, kind, name string, metadata any, items []json.RawMessage) {
	raw, _ := json.Marshal(metadata)
	snapshot := parsedSnapshot{Key: key, Kind: kind, Name: name, Metadata: raw, Ordinals: []int{}}
	for _, item := range items {
		snapshot.Ordinals = append(snapshot.Ordinals, len(part.records))
		part.records = append(part.records, item)
	}
	part.snapshots = append(part.snapshots, snapshot)
}

func csvFields(raw json.RawMessage) map[string]string {
	var row csvRecord
	_ = json.Unmarshal(raw, &row)
	fields := map[string]string{}
	for i, key := range row.Columns {
		if i < len(row.Values) {
			fields[key] = row.Values[i]
		}
	}
	return fields
}

func normalizeSnapshot(raw json.RawMessage, parser string) recordValues {
	v := recordValues{Reason: "music metadata", Origin: "original"}
	if parser == DiscogsCSV || strings.HasPrefix(parser, SpotifyCSV) {
		fields := csvFields(raw)
		if parser == DiscogsCSV {
			v.Artist = fields["Artist"]
			v.Album = fields["Title"]
			if v.Artist == "" {
				v.Artist = fields["artist"]
				v.Album = fields["title"]
			}
			return v
		}
		v.Artist, v.Title, v.Album, v.URI = fields["Artist Name(s)"], fields["Track Name"], fields["Album Name"], fields["Track URI"]
		if v.Artist == "" {
			v.Artist = fields["Album Artist Name(s)"]
		}
		return v
	}
	if parser == SpotifyContext {
		v.Reason = "account context"
		return v
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if parser == SpotifyPlaylist {
		var track map[string]json.RawMessage
		_ = json.Unmarshal(fields["track"], &track)
		fields = track
		v.Artist, v.Title, v.Album, v.URI = rawString(fields["artistName"]), rawString(fields["trackName"]), rawString(fields["albumName"]), rawString(fields["trackUri"])
	} else {
		v.Artist, v.Title, v.Album, v.URI = rawString(fields["artist"]), rawString(fields["track"]), rawString(fields["album"]), rawString(fields["uri"])
	}
	return v
}

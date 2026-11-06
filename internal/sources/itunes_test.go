package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const libraryXML = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Library Persistent ID</key><string>LIBRARY1</string>
<key>Date</key><date>2020-01-02T00:00:00Z</date>
<key>Unknown field</key><dict><key>Flag</key><true/><key>Number</key><integer>9007199254740993</integer><key>Equalizer</key><string>First</string><key>Equalizer</key><string>Second</string></dict>
<key>Tracks</key><dict>
<key>1</key><dict><key>Track ID</key><integer>1</integer><key>Persistent ID</key><string>TRACK1</string><key>Name</key><string>Song &amp; Other</string><key>Artist</key><string>Artist</string><key>Album</key><string>Album</string><key>Play Count</key><integer>7</integer><key>Play Date UTC</key><date>1904-01-01T00:00:00Z</date><key>Date Added</key><date>2019-01-01T00:00:00Z</date></dict>
<key>2</key><dict><key>Track ID</key><integer>2</integer><key>Persistent ID</key><string>TRACK2</string><key>Name</key><string>Episode</string><key>Podcast</key><true/><key>Play Count</key><integer>0</integer></dict>
</dict>
<key>Playlists</key><array><dict><key>Playlist Persistent ID</key><string>PLAYLIST1</string><key>Name</key><string>Playlist</string><key>Playlist Items</key><array><dict><key>Track ID</key><integer>1</integer></dict><dict><key>Track ID</key><integer>1</integer></dict></array></dict></array>
</dict></plist>`

func TestITunesOriginalAndArchiveIdentity(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first, err := s.Import(ctx, bytes.NewBufferString(libraryXML), "library.xml", 1)
	require.NoError(t, err)
	require.Equal(t, 4, first.Added)
	var zipped bytes.Buffer
	z := zip.NewWriter(&zipped)
	w, err := z.Create("renamed.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(libraryXML))
	require.NoError(t, err)
	require.NoError(t, z.Close())
	for _, upload := range []struct {
		data []byte
		name string
	}{{[]byte(libraryXML), "renamed.xml"}, {zipped.Bytes(), "library.zip"}} {
		r, err := s.Import(ctx, bytes.NewReader(upload.data), upload.name, 1)
		require.NoError(t, err)
		require.Zero(t, r.Added)
		require.Equal(t, 4, r.Reused)
	}
	path, err := s.FilePath(first.Hash)
	require.NoError(t, err)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, libraryXML, string(stored))
	var payload, parser, kind string
	err = s.DB.QueryRow(`SELECT payload,parser,kind FROM source_records WHERE ordinal=0`).Scan(&payload, &parser, &kind)
	require.NoError(t, err)
	require.Equal(t, ITunesXML, parser)
	require.Equal(t, "library", kind)
	require.Contains(t, payload, `9007199254740993`)
	require.Contains(t, payload, `"Equalizer":{"$plist_repeated":["First","Second"]}`)
	err = s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='playlist'`).Scan(&payload)
	require.NoError(t, err)
	require.Contains(t, payload, `"Playlist Items":[{"Track ID":1},{"Track ID":1}]`)
	err = s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='library_track' AND ordinal=1`).Scan(&payload)
	require.NoError(t, err)
	require.Contains(t, payload, `"library_id":"LIBRARY1"`)
	require.Contains(t, payload, `"Play Date UTC":"1904-01-01T00:00:00Z"`)
	for i := 0; i < 2; i++ {
		result, err := s.Rebuild(ctx)
		require.NoError(t, err)
		require.Equal(t, 4, result.Records)
		require.Zero(t, result.Plays)
		require.Zero(t, count(t, s, "listens"))
	}
}

func TestITunesConversionAndDerivedEstimates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, input := range []struct{ data, parser, kind string }{
		{`[{"Track ID":1,"Persistent ID":"TRACK1","Play Count":7,"Name":"Song","Artist":"Artist","custom":{"keep":true}}]`, ITunesJSON, "library_track"},
		{`[{"Track ID":[1],"Persistent ID":["TRACK1"],"Play Count":[7],"Name":["Song"],"Artist":["Artist"]}]`, ITunesJSON, "library_track"},
		{`[{"Track ID":1,"Persistent ID":"TRACK1","Play Count":7,"Name":"Song","Artist":"Artist","Estimated Play Time":"2020-01-01T00:00:00Z"},{"Track ID":1,"Persistent ID":"TRACK1","Play Count":7,"Name":"Song","Artist":"Artist","Estimated Play Time":"2020-01-01T00:00:00Z"}]`, ITunesExpanded, "derived_play"},
	} {
		first, err := s.Import(ctx, bytes.NewBufferString(input.data), "data.json", 1)
		require.NoError(t, err)
		repeated, err := s.Import(ctx, bytes.NewBufferString(input.data), "renamed.json", 1)
		require.NoError(t, err)
		require.Equal(t, first.Records, repeated.Reused)
		require.Zero(t, repeated.Added)
		var parser, kind, payload string
		err = s.DB.QueryRow(`SELECT parser,kind,payload FROM source_records WHERE file_hash=? ORDER BY ordinal LIMIT 1`, first.Hash).Scan(&parser, &kind, &payload)
		require.NoError(t, err)
		require.Equal(t, input.parser, parser)
		require.Equal(t, input.kind, kind)
		v := normalizeRecord(json.RawMessage(payload), parser, "UTC")
		require.Equal(t, "Song", v.Title)
		require.True(t, v.Time.IsZero())
		require.NotEqual(t, "counted", v.Reason)
	}
	require.Equal(t, 4, count(t, s, "source_records"))
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
}

func TestITunesMalformedLibrariesRetainFiles(t *testing.T) {
	for _, data := range []string{
		`<plist><dict><key>Tracks</key><dict></dict>`,
		`<plist><dict><key>Tracks</key><dict/><key>Tracks</key><dict/></dict></plist>`,
		`<plist><dict><key>Tracks</key><dict/><key>Unknown</key><uid>2</uid></dict></plist>`,
		`<plist><dict><key>Tracks</key><dict/><key>Unknown</key><string><dict/></string></dict></plist>`,
		`{"Tracks":{},"Playlists":[null]}`,
		`[{"Track ID":1,"Persistent ID":"TRACK1"},null]`,
	} {
		t.Run(data, func(t *testing.T) {
			s := testStore(t)
			result, err := s.Import(context.Background(), bytes.NewBufferString(data), "broken.data", 1)
			require.Error(t, err)
			require.NotEmpty(t, result.Hash)
			require.Zero(t, count(t, s, "source_records"))
			path, err := s.FilePath(result.Hash)
			require.NoError(t, err)
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, data, string(stored))
		})
	}
}

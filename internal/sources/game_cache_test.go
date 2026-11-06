package sources

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// Small anonymous format fixtures keep cache coverage in the ordinary suite.
// Private exports provide the separate full-file acceptance checks.
type cacheFixtureWriter struct {
	bytes.Buffer
	t *testing.T
}

func (w *cacheFixtureWriter) put(value any) {
	w.t.Helper()
	require.NoError(w.t, binary.Write(&w.Buffer, binary.LittleEndian, value))
}
func (w *cacheFixtureWriter) text(value string) {
	for size := uint32(len(value)); ; size >>= 7 {
		if size < 128 {
			w.put(uint8(size))
			break
		}
		w.put(uint8(size) | 128)
	}
	w.WriteString(value)
}
func (w *cacheFixtureWriter) block(value *cacheFixtureWriter) {
	w.put(uint32(value.Len()))
	w.Write(value.Bytes())
}

func yargCacheFixture(t *testing.T, version uint32) []byte {
	out := &cacheFixtureWriter{t: t}
	out.put(version)
	out.put(uint8(0))
	values := []string{"Song", "Artist", "Album", "Genre"}
	if version == 26042800 {
		values = append(values, "Subgenre")
	}
	values = append(values, "2020", "Charter", "Playlist", "Source")
	for _, value := range values {
		block := &cacheFixtureWriter{t: t}
		block.put(uint32(1))
		block.text(value)
		out.block(block)
	}
	for section := 0; section < 5; section++ {
		if section == 1 {
			out.put(uint32(1))
			opaque := &cacheFixtureWriter{t: t}
			opaque.Write([]byte{2, 3, 4})
			out.block(opaque)
			continue
		}
		if section != 3 {
			out.put(uint32(0))
			continue
		}
		out.put(uint32(1))
		group, node := &cacheFixtureWriter{t: t}, &cacheFixtureWriter{t: t}
		group.text("C:/Songs")
		group.put(uint32(1)) // One unpacked INI chart, followed by one SNG chart.
		node.text("Artist/Song")
		node.put(uint8(0))
		node.put(uint64(123))
		node.put(uint8(0))
		metadataStart := node.Len()
		node.Write(bytes.Repeat([]byte{1}, 20))
		node.Write(make([]byte, 42))
		for range values {
			node.put(uint32(0))
		}
		node.put(uint8(1))
		node.put(uint8(0))
		node.put(uint32(2))
		node.put(uint32(3))
		node.put(uint64(180123))
		node.put(uint64(0))
		node.put(uint32(3))
		for i := 0; i < 4; i++ {
			node.put(uint64(i))
		}
		textCount := 33
		if version == 26042800 {
			textCount = 40
		}
		for i := 0; i < textCount; i++ {
			node.text("")
		}
		node.put(uint64(0))
		node.put(uint64(0))
		node.put(uint32(0))
		for _, value := range []string{"background.png", "video.mp4", "cover.png"} {
			node.text(value)
		}
		group.block(node)
		group.put(uint32(1))
		sng := &cacheFixtureWriter{t: t}
		sng.text("Artist/Song.sng")
		sng.put(uint64(456))
		sng.put(uint32(1))
		sng.put(uint8(0))
		sng.Write(node.Bytes()[metadataStart:])
		group.block(sng)
		out.block(group)
	}
	return out.Bytes()
}

func cloneCacheFixture(t *testing.T) []byte {
	out := &cacheFixtureWriter{t: t}
	out.put(uint32(20221120))
	out.Write(make([]byte, 16))
	for i, value := range []string{"Song", "Artist", "Album", "Genre", "2020", "Charter", "Playlist"} {
		out.put(uint8(i))
		out.put(uint32(1))
		out.text(value)
	}
	out.put(uint32(1))
	out.text("C:/Songs/Artist/Song")
	out.put(uint64(123))
	out.put(uint64(456))
	out.text("notes.chart")
	out.put(uint8(0))
	for i := 0; i < 7; i++ {
		out.put(uint32(0))
	}
	out.put(uint64(1))
	out.put(uint8(1))
	out.Write(make([]byte, 12))
	out.put(uint32(15000))
	out.text("example")
	out.put(uint16(2))
	out.put(uint16(3))
	out.Write([]byte{0, 1, 0, 0})
	out.put(uint32(180123))
	out.put(uint64(789))
	out.text("Favorites")
	out.text("Example")
	out.Write(bytes.Repeat([]byte{1}, 16))
	return out.Bytes()
}

func TestAnonymousGameCachesPreserveMetadataAndRejectTruncation(t *testing.T) {
	for name, data := range map[string][]byte{"yarg-nightly": yargCacheFixture(t, 25031401), "yarg-release": yargCacheFixture(t, 26042800), "clone": cloneCacheFixture(t)} {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			first, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
			require.NoError(t, err)
			expected := 4
			if name == "clone" {
				expected = 2
			}
			require.Equal(t, expected, first.Records)
			var parser, payload string
			require.NoError(t, s.DB.QueryRow(`SELECT parser,payload FROM source_records WHERE kind='catalog_track'`).Scan(&parser, &payload))
			v := normalizeRecord([]byte(payload), parser, "")
			require.Equal(t, "Song", v.Title)
			require.Equal(t, "Artist", v.Artist)
			require.Equal(t, "Album", v.Album)
			require.EqualValues(t, 180123, v.DurationMS)
			require.NotEmpty(t, v.NativeTrackID)
			require.Contains(t, payload, `"bytes":`)
			if parser == YARGCache {
				var sng, opaque string
				require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE json_extract(payload,'$.fields.sng_version')=1`).Scan(&sng))
				require.EqualValues(t, 180123, normalizeRecord([]byte(sng), parser, "").DurationMS)
				require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='game_cache_context'`).Scan(&opaque))
				require.Contains(t, opaque, `"bytes":"AgME"`)
			}
			repeat, err := s.Import(ctx, bytes.NewReader(data), "copy.bin", 1)
			require.NoError(t, err)
			require.Zero(t, repeat.Added)
			_, err = s.Import(ctx, bytes.NewReader(data[:len(data)-1]), "truncated.bin", 1)
			require.Error(t, err)
			require.Equal(t, expected, count(t, s, "source_records"))
			result, err := s.Rebuild(ctx)
			require.NoError(t, err)
			require.Zero(t, result.Plays)
		})
	}
}

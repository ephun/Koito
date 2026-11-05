package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/catalog"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/mbz"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDisplayedRecordingIdentifierDoesNotReidentifyIncomingNativePlays(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	old := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	incoming := uuid.MustParse("d9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	track := musicAppearance(t, s, "Song", 1, old)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, UserID: 1, Time: time.Unix(1577934065, 0)}))
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	original := state.Current.Config
	for i, r := range state.Current.Config.Rules {
		if v, ok := state.Blocks.Legacy[r.ID]; ok {
			state.Current.Config.Rules[i] = v
		}
	}
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},{"op":"eq","args":[{"op":"field","field":"catalog.id"},{"op":"literal","value":%d}]}]},"then":[{"set":"music.identifier","value":{"op":"literal","value":%q}}]}]}`, track.ID, incoming.String())), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "display-id", Type: "matching", Subtype: "music", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	view, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
	require.NoError(t, err)
	require.Equal(t, incoming, *view.MbzID)
	raw, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID, Original: true})
	require.NoError(t, err)
	require.Equal(t, old, *raw.MbzID)
	for i, id := range []uuid.UUID{incoming, old} {
		require.NoError(t, catalog.SubmitListen(ctx, s, catalog.SubmitListenOpts{Artist: "Artist", ArtistNames: []string{"Artist"}, TrackTitle: "Song", ReleaseTitle: "Single", RecordingMbzID: id, UserID: 1, Time: time.Unix(1578020465+int64(i)*86400, 0), MbzCaller: &mbz.MbzErrorCaller{}, SkipCacheImage: true}))
	}
	check := func() {
		var recordings, plays int
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(DISTINCT tr.recording_key) FROM listens l JOIN track_recordings tr ON tr.track_id=l.track_id`).Scan(&recordings))
		require.Equal(t, 2, recordings)
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&plays))
		require.Equal(t, 3, plays)
	}
	check()
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check()
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: original})
	require.NoError(t, err)
	check()
}

func TestOriginalCatalogReadsBypassDisplayedNamesIdentifiersCreditsAndClassification(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	old := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	shown := "d9c172fb-2ae4-44d8-95bb-a84c50a5c591"
	track := musicAppearance(t, s, "Song", 1, old)
	other, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Other"})
	require.NoError(t, err)
	require.NoError(t, s.Exec(`UPDATE artists SET musicbrainz_id=? WHERE id=1;UPDATE releases SET musicbrainz_id=? WHERE id=1`, old.String(), old.String()))
	for _, item := range []struct {
		kind, name string
		id         int32
	}{{"artist", "View artist", 1}, {"album", "View album", 1}, {"track", "View track", track.ID}} {
		require.NoError(t, s.Exec(`INSERT INTO catalog_names(kind,item_id,display_name) VALUES(?,?,?)`, item.kind, item.id, item.name))
		require.NoError(t, s.Exec(`INSERT INTO catalog_identifiers(kind,item_id,identifier) VALUES(?,?,?)`, item.kind, item.id, shown))
		if item.kind != "artist" {
			require.NoError(t, s.Exec(`INSERT INTO catalog_credit_sets(kind,item_id) VALUES(?,?)`, item.kind, item.id))
			require.NoError(t, s.Exec(`INSERT INTO catalog_credits(kind,item_id,artist_id,is_primary,position) VALUES(?,?,?,1,0)`, item.kind, item.id, other.ID))
		}
	}
	require.NoError(t, s.Exec(`INSERT INTO album_classification(album_id,various_artists) VALUES(1,1)`))
	view, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
	require.NoError(t, err)
	require.Equal(t, "View track", view.Title)
	require.Equal(t, other.ID, view.Artists[0].ID)
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1, Original: true})
	require.NoError(t, err)
	require.Equal(t, "Artist", artist.Name)
	require.Equal(t, old, *artist.MbzID)
	require.Contains(t, artist.Aliases, "Artist")
	album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1, Original: true})
	require.NoError(t, err)
	require.Equal(t, "Single", album.Title)
	require.Equal(t, old, *album.MbzID)
	require.False(t, album.VariousArtists)
	require.Equal(t, int32(1), album.Artists[0].ID)
	raw, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID, Original: true})
	require.NoError(t, err)
	require.Equal(t, "Song", raw.Title)
	require.Equal(t, old, *raw.MbzID)
	require.Equal(t, int32(1), raw.Artists[0].ID)
}

func TestConflictingNativeArtistMappingPreservesBothOriginalIdentifiers(t *testing.T) {
	t.Setenv("KOITO_DISABLE_DEEZER", "true")
	t.Setenv("KOITO_DISABLE_MUSICBRAINZ", "true")
	t.Setenv("KOITO_DISABLE_COVER_ART_ARCHIVE", "true")
	s := musicStore(t)
	ctx := context.Background()
	old := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	incoming := uuid.MustParse("d9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	require.NoError(t, s.Exec(`UPDATE artists SET musicbrainz_id=? WHERE id=1`, old.String()))
	artists, err := catalog.AssociateArtists(ctx, s, catalog.AssociateArtistsOpts{ArtistMbidMap: []catalog.ArtistMbidMap{{Artist: "Artist", Mbid: incoming}}, ArtistNames: []string{"Artist"}, ArtistName: "Artist", Mbzc: &mbz.MbzErrorCaller{}, SkipCacheImage: true})
	require.NoError(t, err)
	require.Len(t, artists, 1)
	require.NotEqual(t, int32(1), artists[0].ID)
	var original string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT musicbrainz_id FROM artists WHERE id=1`).Scan(&original))
	require.Equal(t, old.String(), original)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT musicbrainz_id FROM artists WHERE id=?`, artists[0].ID).Scan(&original))
	require.Equal(t, incoming.String(), original)
}

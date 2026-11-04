package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNativeCreditEnrichmentAndCorrectionsPublishAtomically(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	second, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Second, artist"})
	require.NoError(t, err)
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var naming, scope filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[
 {"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},{"op":"eq","args":[{"op":"count","args":[{"op":"field","field":"original.artists"}]},{"op":"literal","value":2}]}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Duet"}}]},
 {"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},{"op":"eq","args":[{"op":"count","args":[{"op":"field","field":"original.artists"}]},{"op":"literal","value":2}]}]},"then":[{"set":"display.album","value":{"op":"literal","value":"Duet release"}}]}
 ]}`), &naming))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"display.title"},{"op":"literal","value":"Duet"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &scope))
	state.Current.Config.Rules = append(state.Current.Config.Rules,
		sources.Rule{ID: "duet", Type: "presentation", Subtype: "naming", Enabled: true, Program: &naming},
		sources.Rule{ID: "scope", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &scope})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	require.NoError(t, s.Exec(`CREATE TRIGGER reject_enrichment BEFORE INSERT ON catalog_names WHEN NEW.kind='track' BEGIN SELECT RAISE(ABORT,'correction rejected'); END`))
	update := db.UpdateTrackOpts{ID: track.ID, AddArtists: []int32{second.ID}, Duration: 321}
	limited, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = s.UpdateTrack(limited, update)
	require.ErrorContains(t, err, "correction rejected")
	var count int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artist_tracks WHERE track_id=? AND artist_id=?`, track.ID, second.ID).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artist_releases WHERE release_id=1 AND artist_id=?`, second.ID).Scan(&count))
	require.Zero(t, count)
	album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "Single", album.Title)
	require.NoError(t, s.Exec(`DROP TRIGGER reject_enrichment`))
	require.NoError(t, s.UpdateTrack(limited, update))
	check := func() {
		got, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
		require.NoError(t, err)
		require.Equal(t, "Duet", got.Title)
		require.Equal(t, 321, int(got.Duration))
		require.Len(t, got.Artists, 2)
		album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1})
		require.NoError(t, err)
		require.Equal(t, "Duet release", album.Title)
		var included bool
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT included FROM track_scope WHERE track_id=?`, track.ID).Scan(&included))
		require.False(t, included)
	}
	check()
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check()
}

func TestNativeIdentifierEnrichmentReevaluatesOriginalIdentifierConditions(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	for i, rule := range state.Current.Config.Rules {
		if migrated, ok := state.Blocks.Legacy[rule.ID]; ok {
			state.Current.Config.Rules[i] = migrated
		}
	}
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},{"op":"eq","args":[{"op":"field","field":"original.identifier"},{"op":"literal","value":""}]}]},"then":[{"set":"music.identifier","value":{"op":"literal","value":"d9c172fb-2ae4-44d8-95bb-a84c50a5c591"}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "identifier", Type: "matching", Subtype: "music", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	before, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "d9c172fb-2ae4-44d8-95bb-a84c50a5c591", before.MbzID.String())
	incoming := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	require.NoError(t, s.UpdateArtist(ctx, db.UpdateArtistOpts{ID: 1, MusicBrainzID: incoming}))
	after, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, incoming.String(), after.MbzID.String())
}

func TestNativeRecordingEnrichmentRetainsExistingSpotifyIdentity(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.Exec(`INSERT INTO recordings(recording_key,spotify_uri) VALUES('spotify:fixture','spotify:track:fixture'); UPDATE track_recordings SET recording_key='spotify:fixture' WHERE track_id=?`, track.ID))
	incoming := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	require.NoError(t, s.UpdateTrack(ctx, db.UpdateTrackOpts{ID: track.ID, MusicBrainzID: incoming}))
	var mbid, uri string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT r.musicbrainz_id,r.spotify_uri FROM track_recordings tr JOIN recordings r ON r.recording_key=tr.recording_key WHERE tr.track_id=?`, track.ID).Scan(&mbid, &uri))
	require.Equal(t, incoming.String(), mbid)
	require.Equal(t, "spotify:track:fixture", uri)
	var preserved string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT spotify_uri FROM recordings WHERE recording_key='spotify:fixture'`).Scan(&preserved))
	require.Equal(t, uri, preserved)
}

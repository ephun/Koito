package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestGroupingEnrichmentRefreshesDefaultsAndPreservesSourceExceptions(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	second, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Second, artist"})
	require.NoError(t, err)
	rows := []string{}
	for i, title := range []string{"Default", "Pinned", "Separate"} {
		rows = append(rows, fmt.Sprintf(`{"ts":"2020-01-0%dT03:04:05Z","ms_played":180001,"master_metadata_track_name":%q,"master_metadata_album_artist_name":"Artist","master_metadata_album_album_name":"Album Deluxe","spotify_track_uri":"spotify:track:%d","reason_end":"trackdone"}`, i+2, title, i))
	}
	_, err = store.Import(ctx, strings.NewReader("["+strings.Join(rows, ",")+"]"), "spotify.json", 1)
	require.NoError(t, err)
	_, err = store.Import(ctx, strings.NewReader(`{"Library Persistent ID":"ONE","Date":"2020-01-10T00:00:00Z","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"Default","Artist":"Artist","Album":"Album Deluxe","Play Count":7},"2":{"Track ID":2,"Persistent ID":"B","Name":"Pinned","Artist":"Artist","Album":"Album Deluxe","Play Count":7},"3":{"Track ID":3,"Persistent ID":"C","Name":"Separate","Artist":"Artist","Album":"Album Deluxe","Play Count":7}}}`), "library.json", 1)
	require.NoError(t, err)
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var native, source filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},{"op":"eq","args":[{"op":"field","field":"original.album"},{"op":"literal","value":"Album Deluxe"}]}]},"then":[{"if":{"op":"eq","args":[{"op":"count","args":[{"op":"field","field":"catalog.artist_ids"}]},{"op":"literal","value":1}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Before"}},{"set":"group.various_artists","value":{"op":"literal","value":true}}],"else":[{"set":"group.album","value":{"op":"literal","value":"After"}}]}]}]}`), &native))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"original.title"},{"op":"literal","value":"Pinned"}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Pinned group"}}],"else_if":[{"if":{"op":"eq","args":[{"op":"field","field":"original.title"},{"op":"literal","value":"Separate"}]},"then":[{"set":"group.album","value":{"op":"literal","value":""}}]}]}]}`), &source))
	state.Current.Config.Rules = append(state.Current.Config.Rules,
		sources.Rule{ID: "count", Type: "inference", Subtype: "count", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{CountMode: "maximum"}},
		sources.Rule{ID: "listener", Type: "attribution", Subtype: "direct", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{Listener: "self"}},
		sources.Rule{ID: "native", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &native},
		sources.Rule{ID: "source", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &source})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	var album int32
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT release_id FROM tracks LIMIT 1`).Scan(&album))
	var rawBefore string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT json_group_array(payload) FROM (SELECT payload FROM source_records ORDER BY id)`).Scan(&rawBefore))
	var undated int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT SUM(undated_count) FROM source_aggregates`).Scan(&undated))
	require.Positive(t, undated)
	check := func(label string, various bool) {
		rows, err := s.SourceDatabase().Query(`SELECT t.title,COALESCE(g.title,'') FROM album_report_events e JOIN tracks_with_title t ON t.id=e.track_id LEFT JOIN album_groups g ON g.id=e.album_group_id ORDER BY t.title`)
		require.NoError(t, err)
		actual := map[string]string{}
		for rows.Next() {
			var title, group string
			require.NoError(t, rows.Scan(&title, &group))
			actual[title] = group
		}
		require.NoError(t, rows.Err())
		rows.Close()
		require.Equal(t, map[string]string{"Default": label, "Pinned": "Pinned group", "Separate": ""}, actual)
		item, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: album})
		require.NoError(t, err)
		require.Equal(t, various, item.VariousArtists)
		var overrides int
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM source_play_groupings`).Scan(&overrides))
		require.Equal(t, 2, overrides)
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM source_aggregate_groupings`).Scan(&overrides))
		require.Equal(t, 2, overrides)
		rows, err = s.SourceDatabase().Query(`SELECT t.title,COALESCE(g.title,'') FROM album_report_counts e JOIN tracks_with_title t ON t.id=e.track_id LEFT JOIN album_groups g ON g.id=e.album_group_id WHERE e.listened_at IS NULL ORDER BY t.title`)
		require.NoError(t, err)
		actual = map[string]string{}
		for rows.Next() {
			var title, group string
			require.NoError(t, rows.Scan(&title, &group))
			actual[title] = group
		}
		require.NoError(t, rows.Err())
		rows.Close()
		require.Equal(t, map[string]string{"Default": label, "Pinned": "Pinned group", "Separate": ""}, actual)
		var amount int
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT SUM(amount) FROM album_report_counts WHERE listened_at IS NULL`).Scan(&amount))
		require.Equal(t, undated, amount)
	}
	check("Before", true)
	var previous int64
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT group_id FROM native_release_groups WHERE release_id=?`, album).Scan(&previous))
	require.NoError(t, s.Exec(`CREATE TRIGGER reject_group_enrichment BEFORE INSERT ON native_release_groups WHEN (SELECT title FROM album_groups WHERE id=NEW.group_id)='After' BEGIN SELECT RAISE(ABORT,'group rejected'); END`))
	err = s.AddArtistsToAlbum(ctx, db.AddArtistsToAlbumOpts{AlbumID: album, ArtistIDs: []int32{second.ID}})
	require.ErrorContains(t, err, "group rejected")
	check("Before", true)
	var additions int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artist_releases WHERE release_id=? AND artist_id=?`, album, second.ID).Scan(&additions))
	require.Zero(t, additions)
	require.NoError(t, s.Exec(`DROP TRIGGER reject_group_enrichment`))
	require.NoError(t, s.AddArtistsToAlbum(ctx, db.AddArtistsToAlbumOpts{AlbumID: album, ArtistIDs: []int32{second.ID}}))
	check("After", false)
	var stale int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_group_members WHERE release_id=? AND group_id=?`, album, previous).Scan(&stale))
	require.Zero(t, stale)
	var canonical *int64
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT canonical_release_id FROM album_groups WHERE id=?`, previous).Scan(&canonical))
	require.Nil(t, canonical)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check("After", false)
	var rawAfter string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT json_group_array(payload) FROM (SELECT payload FROM source_records ORDER BY id)`).Scan(&rawAfter))
	require.Equal(t, rawBefore, rawAfter)
}

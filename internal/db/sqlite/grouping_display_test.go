package sqlite_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestGenreDependentArtistDisplayRefreshesNativeGroupsAndImplicitBaseMembers(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	_, err := store.Import(ctx, strings.NewReader(`[{"ts":"2020-01-02T03:04:05Z","ms_played":180001,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"Artist","master_metadata_album_album_name":"Album (Deluxe)","spotify_track_uri":"spotify:track:one","reason_end":"trackdone"},{"ts":"2020-01-03T03:04:05Z","ms_played":180001,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"Artist","master_metadata_album_album_name":"Album","spotify_track_uri":"spotify:track:one","reason_end":"trackdone"}]`), "spotify.json", 1)
	require.NoError(t, err)
	_, err = store.Import(ctx, strings.NewReader(`{"Library Persistent ID":"ONE","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"Song","Artist":"Artist","Album":"Album","Genre":"Rock","Play Count":7}}}`), "library.json", 1)
	require.NoError(t, err)
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var naming, grouping filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"if":{"op":"in","args":[{"op":"literal","value":"Rock"},{"op":"field","field":"source.genres"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Corrected"}}],"else":[{"set":"display.artist","value":{"op":"field","field":"original.artist"}}]}]}]}`), &naming))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"display.artist"},{"op":"literal","value":"Corrected"}]},{"op":"regex","args":[{"op":"field","field":"display.album"},{"op":"literal","value":"Deluxe"}]}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Album"}},{"set":"group.various_artists","value":{"op":"literal","value":true}}]}]}`), &grouping))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "genre-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: &naming}, sources.Rule{ID: "editions", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &grouping})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	var plays int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_report_events WHERE album_group_id>0`).Scan(&plays))
	require.Equal(t, 2, plays)
	var group int64
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT album_group_id FROM album_report_events LIMIT 1`).Scan(&group))
	require.Positive(t, group)
	incoming, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Album (Deluxe)", ArtistIDs: []int32{1}})
	require.NoError(t, err)
	var incomingGroup int64
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT group_id FROM native_release_groups WHERE release_id=?`, incoming.ID).Scan(&incomingGroup))
	require.Equal(t, group, incomingGroup)
	other, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Incoming"})
	require.NoError(t, err)
	require.NoError(t, s.SaveArtistAliases(ctx, other.ID, []string{"Artist"}, "MusicBrainz"))
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "Artist", artist.Name)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_report_events WHERE album_group_id>0`).Scan(&plays))
	require.Zero(t, plays)
	var members int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_group_members WHERE group_id=?`, group).Scan(&members))
	require.Zero(t, members)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_classification`).Scan(&members))
	require.Zero(t, members)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_report_events WHERE album_group_id>0`).Scan(&plays))
	require.Zero(t, plays)
	var label string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT value FROM source_genres LIMIT 1`).Scan(&label))
	require.Equal(t, "Rock", label)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&plays))
	require.Equal(t, 2, plays)
}

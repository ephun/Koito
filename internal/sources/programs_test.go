package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func namingFixture(t *testing.T, body string) *filters.Program {
	t.Helper()
	var p filters.Program
	require.NoError(t, json.Unmarshal([]byte(body), &p))
	return &p
}

func TestNamingProgramPublishesAndReversesWithoutChangingRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := strings.ReplaceAll(complete, `"Album"`, `"Album (Deluxe)"`)
	mustImport(t, s, []byte("["+data+"]"), "spotify.json", "")
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"display.album","value":{"op":"regex_replace","args":[{"op":"field","field":"original.album"},{"op":"literal","value":" \\(Deluxe\\)$"},{"op":"literal","value":""}]}},{"set":"display.title","value":{"op":"upper","args":[{"op":"field","field":"display.title"}]}}]}]}`)
	for _, enabled := range []bool{true, false, true} {
		change := currentChange(t, s)
		found := false
		for i := range change.Config.Rules {
			if change.Config.Rules[i].ID == "names" {
				change.Config.Rules[i].Enabled = enabled
				found = true
			}
		}
		if !found {
			change.Config.Rules = append(change.Config.Rules, Rule{ID: "names", Name: "Album editions", Type: "presentation", Subtype: "naming", Enabled: enabled, Program: program})
		}
		preview, err := s.Preview(ctx, change)
		require.NoError(t, err)
		require.Len(t, preview.NamingExamples, 2)
		var titleChange NamingExample
		for _, example := range preview.NamingExamples {
			if example.Field == "Track" {
				titleChange = example
			}
		}
		if enabled {
			require.Equal(t, "SONG", titleChange.After)
		} else {
			require.Equal(t, "Song", titleChange.After)
		}
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var title, album, payload, key string
		require.NoError(t, s.DB.QueryRow(`SELECT t.title,a.title,r.payload,p.play_key FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id JOIN releases_with_title a ON a.id=t.release_id JOIN source_records r ON r.id=p.representative_record`).Scan(&title, &album, &payload, &key))
		if enabled {
			require.Equal(t, "SONG", title)
			require.Equal(t, "Album", album)
		} else {
			require.Equal(t, "Song", title)
			require.Equal(t, "Album (Deluxe)", album)
		}
		require.JSONEq(t, data, payload)
		require.Equal(t, 1, count(t, s, "listens"))
		require.Equal(t, 1, count(t, s, "source_play_records"))
		if _, err = s.Rebuild(ctx); err != nil {
			t.Fatal(err)
		}
		var after string
		require.NoError(t, s.DB.QueryRow(`SELECT play_key FROM source_plays`).Scan(&after))
		require.Equal(t, key, after)
	}
}

func TestInvalidNamingChangeRetainsUsableViewAndSavedVersion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	before := currentChange(t, s)
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "empty", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"display.title","value":{"op":"literal","value":""}}]}`)})
	_, err = s.Save(ctx, change)
	require.ErrorContains(t, err, "empty artist or title")
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	require.Equal(t, before.BaseVersion, state.Current.ID)
	var title string
	require.NoError(t, s.DB.QueryRow(`SELECT t.title FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id`).Scan(&title))
	require.Equal(t, "Song", title)
	require.Equal(t, 1, count(t, s, "source_records"))
	require.Equal(t, 1, count(t, s, "listens"))
}

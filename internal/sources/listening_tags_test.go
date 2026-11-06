package sources

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestListeningTagsRemainPerPlayAndReverseWithoutChangingRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first := strings.TrimSuffix(complete, "}") + `,"custom_tag":"First"}`
	second := strings.Replace(strings.Replace(first, "First", "Second", 1), "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z", 1)
	mustImport(t, s, []byte("["+first+","+second+"]"), "spotify.json", "")
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "tags", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"display.tags","value":{"op":"list","args":[{"op":"field","field":"source.service"},{"op":"field","field":"raw:/custom_tag"}]}}]}`)})
	for _, enabled := range []bool{true, false, true} {
		change.BaseVersion = currentChange(t, s).BaseVersion
		change.Config.Rules[len(change.Config.Rules)-1].Enabled = enabled
		preview, err := s.Preview(ctx, change)
		require.NoError(t, err)
		require.NotEmpty(t, preview.NamingExamples)
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		if enabled {
			rows, err := s.DB.Query(`SELECT tags.tags_json FROM source_play_tags tags JOIN source_plays p ON p.play_key=tags.play_key ORDER BY p.listened_at`)
			require.NoError(t, err)
			actual := [][]string{}
			for rows.Next() {
				var raw string
				require.NoError(t, rows.Scan(&raw))
				var tags []string
				require.NoError(t, json.Unmarshal([]byte(raw), &tags))
				actual = append(actual, tags)
			}
			rows.Close()
			require.Equal(t, [][]string{{"spotify", "First"}, {"spotify", "Second"}}, actual)
		} else {
			require.Zero(t, count(t, s, "source_play_tags"))
		}
		require.Equal(t, 2, count(t, s, "listens"))
		require.Equal(t, 2, count(t, s, "source_records"))
		var payload string
		require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE ordinal=0`).Scan(&payload))
		require.JSONEq(t, first, payload)
	}
}

func TestInvalidTagsDoNotPublishPartialNamingChanges(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	original := currentChange(t, s)
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "invalid-tags", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"display.title","value":{"op":"literal","value":"Changed"}},{"set":"display.tags","value":{"op":"list","args":[{"op":"literal","value":12}]}}]}`)})
	_, err = s.Save(ctx, change)
	require.ErrorContains(t, err, "nonempty text")
	require.Equal(t, original.BaseVersion, currentChange(t, s).BaseVersion)
	var title string
	require.NoError(t, s.DB.QueryRow(`SELECT t.title FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id`).Scan(&title))
	require.Equal(t, "Song", title)
	require.Zero(t, count(t, s, "source_play_tags"))
	require.Equal(t, 1, count(t, s, "source_records"))
}

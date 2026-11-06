package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
	"github.com/stretchr/testify/require"
)

func TestEditionGroupingTemplatePreservesVersionMeaningAndIsOptIn(t *testing.T) {
	p, err := filters.CompileProgram(*editionGroupingProgram(), music.GroupingSchema())
	require.NoError(t, err)
	for _, item := range []struct{ title, want string }{{"Album (Deluxe)", "Album"}, {"Album [Expanded Edition]", "Album"}, {"Album (Remastered 2020)", "Album"}, {"Album (Live)", ""}, {"Album", ""}} {
		out, err := p.Run(context.Background(), filters.Env{"display.album": filters.String(item.title)})
		require.NoError(t, err)
		require.Empty(t, out.Issues)
		if item.want == "" {
			require.Empty(t, out.Values)
		} else {
			actual, _ := out.Values["group.album"].Text()
			require.Equal(t, item.want, actual)
		}
	}
}

func TestUndatedContributionsOnOneAppearanceRetainIndependentGroups(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	importLibrarySnapshot(t, s, "TWO", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 3)})
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, countRules()...)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "one", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"library.id"},{"op":"literal","value":"ONE"}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Grouped"}}]}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	var grouped, ungrouped, tracks int
	require.NoError(t, s.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM album_report_counts WHERE album_group_id>0`).Scan(&grouped))
	require.Equal(t, 7, grouped)
	require.NoError(t, s.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM album_report_counts WHERE album_group_id=0`).Scan(&ungrouped))
	require.Equal(t, 3, ungrouped)
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT track_id) FROM album_report_counts`).Scan(&tracks))
	require.Equal(t, 1, tracks)
	require.Equal(t, 0, count(t, s, "listens"))
}

func TestAlbumGroupingRetainsReleasesAndSourceExceptionsAndReverses(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	deluxe := strings.ReplaceAll(complete, `"Album"`, `"Album (Deluxe)"`)
	base := strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z")
	mustImport(t, s, []byte("["+deluxe+","+base+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album (Deluxe)", 1578106865)+"]")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	before := map[int64]string{}
	rows, err := s.DB.Query(`SELECT id,payload FROM source_records`)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		var raw string
		require.NoError(t, rows.Scan(&id, &raw))
		before[id] = raw
	}
	rows.Close()
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "editions", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"group.album","value":{"op":"regex_replace","args":[{"op":"field","field":"original.album"},{"op":"literal","value":" \\(Deluxe\\)$"},{"op":"literal","value":""}]}}]}]}`)})
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.Contains(t, preview.GroupingExamples, NamingExample{RecordID: 1, Field: "Album group", Before: "Album (Deluxe)", After: "Album"})
	var groupID int64
	for _, enabled := range []bool{true, false, true} {
		change = currentChange(t, s)
		found := false
		for i := range change.Config.Rules {
			if change.Config.Rules[i].ID == "editions" {
				change.Config.Rules[i].Enabled = enabled
				found = true
			}
		}
		if !found {
			change.Config.Rules = append(change.Config.Rules, Rule{ID: "editions", Type: "presentation", Subtype: "grouping", Enabled: enabled, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"group.album","value":{"op":"regex_replace","args":[{"op":"field","field":"original.album"},{"op":"literal","value":" \\(Deluxe\\)$"},{"op":"literal","value":""}]}}]}]}`)})
		}
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var groups, grouped, ungrouped, releases int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT album_group_id) FROM album_report_events WHERE album_group_id>0`).Scan(&groups))
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM album_report_events WHERE album_group_id>0`).Scan(&grouped))
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM album_report_events WHERE album_group_id=0`).Scan(&ungrouped))
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT release_id) FROM album_report_events`).Scan(&releases))
		require.Equal(t, 2, releases)
		if enabled {
			require.Equal(t, 1, groups)
			require.Equal(t, 2, grouped)
			require.Equal(t, 1, ungrouped)
			var id int64
			require.NoError(t, s.DB.QueryRow(`SELECT id FROM album_groups WHERE title='Album'`).Scan(&id))
			if groupID > 0 {
				require.Equal(t, groupID, id)
			}
			groupID = id
			var title string
			require.NoError(t, s.DB.QueryRow(`SELECT r.title FROM album_groups g JOIN releases_with_title r ON r.id=g.canonical_release_id WHERE g.id=?`, id).Scan(&title))
			require.Equal(t, "Album", title)
		} else {
			require.Equal(t, 0, groups)
			require.Equal(t, 0, grouped)
			require.Equal(t, 3, ungrouped)
		}
		for id, raw := range before {
			var after string
			require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE id=?`, id).Scan(&after))
			require.Equal(t, raw, after)
		}
		_, err = s.Rebuild(ctx)
		require.NoError(t, err)
	}
}

func TestGroupingUnresolvedFilterIsAtomicAndEmptyAssignmentIsException(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "all", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"group.album","value":{"op":"literal","value":"Grouped"}}]}`)},
		Rule{ID: "unknown", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"group.album","value":{"op":"literal","value":"Wrong"}},{"if":{"op":"eq","args":[{"op":"field","field":"raw:/missing"},{"op":"literal","value":true}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Also wrong"}}]}]}`)},
		Rule{ID: "except", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"group.album","value":{"op":"literal","value":""}}]}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	var group int
	require.NoError(t, s.DB.QueryRow(`SELECT album_group_id FROM album_report_events`).Scan(&group))
	require.Zero(t, group)
	inspected, err := s.Inspect(ctx, 1)
	require.NoError(t, err)
	require.NotEmpty(t, inspected.FilterIssues)
	var wrong int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM album_groups WHERE title='Wrong'`).Scan(&wrong))
	require.Zero(t, wrong)
}

package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupingPreviewAndPublicationUseCorrectedCatalogDisplays(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	deluxe := strings.ReplaceAll(complete, `"Album"`, `"Original (Deluxe)"`)
	base := strings.ReplaceAll(strings.ReplaceAll(complete, `"Album"`, `"Original"`), "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z")
	mustImport(t, s, []byte("["+deluxe+","+base+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	change := currentChange(t, s)
	original := change.Config
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "names", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},"then":[{"set":"display.album","value":{"op":"regex_replace","args":[{"op":"field","field":"original.album"},{"op":"literal","value":"^Original"},{"op":"literal","value":"Renamed"}]}}],"else_if":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Corrected artist"}}]}]}]}`)},
		Rule{ID: "editions", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"display.artist"},{"op":"literal","value":"Corrected artist"}]},{"op":"regex","args":[{"op":"field","field":"display.album"},{"op":"literal","value":"\\s*\\(Deluxe\\)$"}]}]},"then":[{"set":"group.album","value":{"op":"regex_replace","args":[{"op":"field","field":"display.album"},{"op":"literal","value":"\\s*\\(Deluxe\\)$"},{"op":"literal","value":""}]}},{"set":"group.various_artists","value":{"op":"literal","value":true}}]}]}`)})
	versions := count(t, s, "filter_versions")
	_, err = s.DB.Exec(`CREATE TRIGGER reject_display_preview BEFORE INSERT ON native_release_groups BEGIN SELECT RAISE(ABORT,'group rejected'); END`)
	require.NoError(t, err)
	_, err = s.Preview(ctx, change)
	require.ErrorContains(t, err, "group rejected")
	require.Equal(t, versions, count(t, s, "filter_versions"))
	require.Zero(t, count(t, s, "catalog_names"))
	require.Zero(t, count(t, s, "native_release_groups"))
	require.Equal(t, 2, count(t, s, "listens"))
	_, err = s.DB.Exec(`DROP TRIGGER reject_display_preview`)
	require.NoError(t, err)
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.Contains(t, preview.GroupingExamples, NamingExample{RecordID: 1, Field: "Album group", Before: "Original (Deluxe)", After: "Renamed"})
	require.Contains(t, preview.GroupingExamples, NamingExample{Field: "Various Artists: Original (Deluxe)", Before: "false", After: "true"})
	require.Equal(t, versions, count(t, s, "filter_versions"))
	require.Zero(t, count(t, s, "catalog_names"))
	require.Zero(t, count(t, s, "native_release_groups"))
	require.Equal(t, 2, count(t, s, "listens"))
	change.Token = preview.Token
	_, err = s.Apply(ctx, change)
	require.NoError(t, err)
	var grouped int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM album_report_events e JOIN album_groups g ON g.id=e.album_group_id WHERE g.title='Renamed'`).Scan(&grouped))
	require.Equal(t, 2, grouped)
	var canonical string
	require.NoError(t, s.DB.QueryRow(`SELECT r.title FROM album_groups g JOIN releases_with_title r ON r.id=g.canonical_release_id WHERE g.title='Renamed' AND EXISTS(SELECT 1 FROM album_group_members m WHERE m.group_id=g.id)`).Scan(&canonical))
	require.Equal(t, "Original", canonical)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM album_report_events e JOIN album_groups g ON g.id=e.album_group_id WHERE g.title='Renamed'`).Scan(&grouped))
	require.Equal(t, 2, grouped)
	change = currentChange(t, s)
	change.Config = original
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "native_release_groups"))
	require.Zero(t, count(t, s, "album_classification"))
	require.Zero(t, count(t, s, "catalog_names"))
	require.Equal(t, 2, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "listens"))
}

func TestAlbumGroupingFollowsReversibleArtistCreditCorrections(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	other := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(complete, `"Artist"`, `"ARTIST"`), `"Album"`, `"ALBUM"`), "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z")
	mustImport(t, s, []byte("["+complete+","+other+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	change := currentChange(t, s)
	original := change.Config
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "credits", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"or","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]}]},"then":[{"set":"display.artist_ids","value":{"op":"literal","value":[1]}},{"set":"display.primary_artist_ids","value":{"op":"literal","value":[1]}}]}]}`)},
		Rule{ID: "group", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"group.album","value":{"op":"casefold","args":[{"op":"field","field":"display.album"}]}}]}`)})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	var groups, artists int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT album_group_id) FROM album_report_events`).Scan(&groups))
	require.Equal(t, 1, groups)
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT a.artist_id) FROM displayed_artist_tracks a JOIN listens l ON l.track_id=a.track_id`).Scan(&artists))
	require.Equal(t, 1, artists)
	change = currentChange(t, s)
	change.Config = original
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT a.artist_id) FROM artist_tracks a JOIN listens l ON l.track_id=a.track_id`).Scan(&artists))
	require.Equal(t, 2, artists)
	require.Equal(t, 2, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "listens"))
}

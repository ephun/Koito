package sources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCombinedCatalogNamesAndGenresShareOrderedAtomicResults(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	track := libraryFixtureTrack("A", 7)
	track["Genre"] = "Rock"
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{track})
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	change := currentChange(t, s)
	original := change.Config
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "combined", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Clean"}},{"set":"artist.genres","value":{"op":"list","args":[{"op":"field","field":"display.artist"}]}}]}]}`)},
		Rule{ID: "later", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},{"op":"in","args":[{"op":"literal","value":"Clean"},{"op":"field","field":"artist.genres"}]}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Final"}}]}]}`)})
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.Contains(t, preview.NamingExamples, NamingExample{Field: "Artist name", Before: "Artist", After: "Final"})
	require.Contains(t, preview.NamingExamples, NamingExample{Field: "Artist genres", Before: `["Rock"]`, After: `["Clean"]`})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	check := func(id int) {
		var name, genre string
		require.NoError(t, s.DB.QueryRow(`SELECT display_name FROM catalog_names WHERE kind='artist' AND item_id=?`, id).Scan(&name))
		require.Equal(t, "Final", name)
		require.NoError(t, s.DB.QueryRow(`SELECT genre FROM artist_genres WHERE artist_id=?`, id).Scan(&genre))
		require.Equal(t, "Clean", genre)
	}
	check(1)
	change = currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "invalid", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Wrong"}},{"set":"artist.genres","value":{"op":"literal","value":[""]}}]}]}`)})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	check(1)
	var issues int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM artist_genre_issues WHERE artist_id=1`).Scan(&issues))
	require.Equal(t, 1, issues)
	change = currentChange(t, s)
	change.Config.Rules[len(change.Config.Rules)-1].Program = namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":""}},{"set":"artist.genres","value":{"op":"literal","value":["Wrong"]}}]}]}`)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	check(1)
	tx, err := s.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases(artist_id,alias,source,is_primary) VALUES(2,'Incoming','Canonical',1)`)
	require.NoError(t, err)
	require.NoError(t, RefreshNativeCatalog(ctx, tx, "artist", 2))
	require.NoError(t, tx.Commit())
	check(2)
	check(1)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	check(1)
	check(2)
	change = currentChange(t, s)
	change.Config = original
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	var name, genre string
	require.NoError(t, s.DB.QueryRow(`SELECT name FROM artists_with_name WHERE id=1`).Scan(&name))
	require.Equal(t, "Artist", name)
	require.NoError(t, s.DB.QueryRow(`SELECT genre FROM artist_genres WHERE artist_id=1`).Scan(&genre))
	require.Equal(t, "Rock", genre)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 3, count(t, s, "source_records"))
}

func TestOneNamingFilterBranchesBetweenRecordsAndArtists(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "contexts", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"record"}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Changed song"}}],"else_if":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Changed artist"}},{"set":"artist.genres","value":{"op":"literal","value":["Jazz"]}}]}]}]}`)})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	var name, genre, title string
	require.NoError(t, s.DB.QueryRow(`SELECT display_name FROM catalog_names WHERE kind='artist' AND item_id=1`).Scan(&name))
	require.Equal(t, "Changed artist", name)
	require.NoError(t, s.DB.QueryRow(`SELECT genre FROM artist_genres WHERE artist_id=1`).Scan(&genre))
	require.Equal(t, "Jazz", genre)
	require.NoError(t, s.DB.QueryRow(`SELECT t.title FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id`).Scan(&title))
	require.Equal(t, "Changed song", title)
	inspection, err := s.Inspect(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, inspection.FilterIssues)
	change = currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "invalid-record-genres", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"record"}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Wrong"}},{"set":"artist.genres","value":{"op":"literal","value":["Wrong"]}}]}]}`)})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.NoError(t, s.DB.QueryRow(`SELECT t.title FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id`).Scan(&title))
	require.Equal(t, "Changed song", title)
	inspection, err = s.Inspect(ctx, 1)
	require.NoError(t, err)
	require.Len(t, inspection.FilterIssues, 1)
	require.Equal(t, "artist.genres", inspection.FilterIssues[0].Path)
	require.Equal(t, 1, count(t, s, "source_records"))
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestSharedAliasRefreshesGenreDependentNamesForOtherArtists(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	track := libraryFixtureTrack("A", 7)
	track["Genre"] = "Rock"
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{track})
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "genre-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"artist.genres","value":{"op":"field","field":"source.genres"}},{"if":{"op":"in","args":[{"op":"literal","value":"Rock"},{"op":"field","field":"artist.genres"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Corrected"}}],"else":[{"set":"display.artist","value":{"op":"field","field":"original.artist"}}]}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	var name string
	require.NoError(t, s.DB.QueryRow(`SELECT display_name FROM catalog_names WHERE kind='artist' AND item_id=1`).Scan(&name))
	require.Equal(t, "Corrected", name)
	tx, err := s.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases(artist_id,alias,source,is_primary) VALUES(2,'Incoming','Canonical',1),(2,'Artist','MusicBrainz',0)`)
	require.NoError(t, err)
	require.NoError(t, RefreshNativeCatalog(ctx, tx, "artist", 2))
	require.NoError(t, tx.Commit())
	require.NoError(t, s.DB.QueryRow(`SELECT COALESCE((SELECT display_name FROM catalog_names WHERE kind='artist' AND item_id=a.id),a.name) FROM artists_with_name a WHERE id=1`).Scan(&name))
	require.Equal(t, "Artist", name)
	require.Zero(t, count(t, s, "artist_genres"))
	var label string
	require.NoError(t, s.DB.QueryRow(`SELECT value FROM source_genres LIMIT 1`).Scan(&label))
	require.Equal(t, "Rock", label)
}

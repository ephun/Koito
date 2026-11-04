package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestSourceFieldsPreserveAbsentNullEmptyZeroAndFalse(t *testing.T) {
	root, err := decodeRaw(`{"null":null,"empty":"","zero":0,"false":false,"a/b":{"~key":[9007199254740993]}}`)
	require.NoError(t, err)
	for path, kind := range map[string]filters.Type{"raw:/absent": filters.Missing, "raw:/null": filters.Null, "raw:/empty": filters.Text, "raw:/zero": filters.Number, "raw:/false": filters.Boolean, "raw:": filters.Object, "raw:/a~1b/~0key": filters.List} {
		require.Equal(t, kind, rawValue(root, path).Type, path)
	}
	n, ok := rawValue(root, "raw:/a~1b/~0key/0").Int64()
	require.True(t, ok)
	require.EqualValues(t, 9007199254740993, n)
	require.Equal(t, filters.Missing, rawValue(root, "raw:/a~1b/~0key/1").Type)
	csv, err := decodeRaw(`{"columns":["Artist","a/b~c","Empty","Zero"],"values":["Example","kept","","0"]}`)
	require.NoError(t, err)
	for field, want := range map[string]string{"column:/Artist": "Example", "column:/a~1b~0c": "kept", "column:/Empty": "", "column:/Zero": "0"} {
		value, ok := rawValue(csv, field).Text()
		require.True(t, ok)
		require.Equal(t, want, value)
	}
	require.Equal(t, filters.Missing, rawValue(csv, "column:/absent").Type)
}

func TestSourceFieldCatalogUsesAllRecordsAndRefreshesAfterImport(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := strings.TrimSuffix(complete, "}") + `,"variable":null,"playlist":[{"name":"Example"}],"columns":["Genre"],"values":["Rock"]}`
	mustImport(t, s, []byte("["+data+"]"), "first.json", "")
	fields, err := s.SourceFields(ctx, 1)
	require.NoError(t, err)
	paths := map[string][]filters.Type{}
	for _, field := range fields {
		paths[field.Path] = append(paths[field.Path], field.Type)
	}
	require.Equal(t, []filters.Type{filters.Null}, paths["raw:/variable"])
	require.Equal(t, []filters.Type{filters.List}, paths["raw:/playlist"])
	require.Equal(t, []filters.Type{filters.Text}, paths["column:/Genre"])
	require.NotContains(t, paths, "raw:/playlist/0/name")
	cached, err := s.SourceFields(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, fields, cached)
	second := strings.TrimSuffix(complete, "}") + `,"variable":false,"new_field":42}`
	mustImport(t, s, []byte("["+second+"]"), "second.json", "")
	updated, err := s.SourceFields(ctx, 1)
	require.NoError(t, err)
	paths = map[string][]filters.Type{}
	for _, field := range updated {
		paths[field.Path] = append(paths[field.Path], field.Type)
	}
	require.ElementsMatch(t, []filters.Type{filters.Null, filters.Boolean}, paths["raw:/variable"])
	require.Equal(t, []filters.Type{filters.Number}, paths["raw:/new_field"])
	_, err = s.SourceFields(ctx, 999)
	require.ErrorContains(t, err, "no records")
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE id=1`).Scan(&payload))
	require.JSONEq(t, data, payload)
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestRawMetadataDrivesReversibleNamingAndDirectAttribution(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := strings.TrimSuffix(complete, "}") + `,"custom":{"genre":"Jazz","mine":false}}`
	mustImport(t, s, []byte("["+data+"]"), "history.json", "")
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "raw-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"raw:/custom/genre"},{"op":"literal","value":"Jazz"}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Changed"}}]}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	var title string
	require.NoError(t, s.DB.QueryRow(`SELECT title FROM tracks_with_title WHERE id=(SELECT track_id FROM source_plays)`).Scan(&title))
	require.Equal(t, "Changed", title)
	change = currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].ID == "raw-name" {
			change.Config.Rules[i].Enabled = false
		}
	}
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.Contains(t, preview.NamingExamples, NamingExample{RecordID: 1, Field: "Track", Before: "Changed", After: "Song"})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	change = currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "raw-owner", Type: "attribution", Subtype: "direct", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"raw:/custom/mine"},{"op":"literal","value":false}]},"then":[{"set":"listener","value":{"op":"literal","value":"other"}}]}]}`)})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 0, count(t, s, "listens"))
	change = currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].ID == "raw-owner" {
			change.Config.Rules[i].Enabled = false
		}
	}
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records`).Scan(&payload))
	require.JSONEq(t, data, payload)
}

func TestInspectionExplainsUnresolvedSourceFieldsAndConditionValues(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "history.json", "")
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "missing", Name: "Imported genre", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"raw:/Genre"},{"op":"literal","value":"Jazz"}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Changed"}}]}]}`)},
		Rule{ID: "owner", Name: "Imported listener", Type: "attribution", Subtype: "direct", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"listener","value":{"op":"field","field":"raw:/owner"}}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	inspection, err := s.Inspect(ctx, 1)
	require.NoError(t, err)
	require.Len(t, inspection.FilterIssues, 2)
	require.Equal(t, "Imported genre", inspection.FilterIssues[0].Name)
	require.Equal(t, "steps.0.if", inspection.FilterIssues[0].Path)
	require.Equal(t, "Missing field: raw:/Genre", inspection.FilterIssues[0].Reason)
	require.Equal(t, "Imported listener", inspection.FilterIssues[1].Name)
	require.Equal(t, "Missing field: raw:/owner", inspection.FilterIssues[1].Reason)
	require.JSONEq(t, complete, inspection.Record.Payload)
}

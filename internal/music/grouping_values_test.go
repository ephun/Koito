package music

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestGroupingActionsShareStateAndRejectPartialRecordChanges(t *testing.T) {
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"literal","value":true},"then":[{"set":"group.album","value":{"op":"literal","value":"Base"}},{"set":"group.various_artists","value":{"op":"eq","args":[{"op":"field","field":"group.album"},{"op":"literal","value":"Base"}]}}]}]}`), &program))
	programs, err := CompileGroupingPrograms([]IdentityFilter{{ID: "mixed", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &program}})
	require.NoError(t, err)
	result, issues, err := programs.ResolveValues(context.Background(), AlbumGroupingEnvironment(1, "Edition", false))
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, "Base", result.Album)
	require.True(t, result.AlbumAssigned)
	require.True(t, result.VariousArtists)
	require.True(t, result.VariousAssigned)
	result, issues, err = programs.ResolveValues(context.Background(), filters.Env{"catalog.kind": filters.String("record")})
	require.NoError(t, err)
	require.NotEmpty(t, issues)
	require.False(t, result.AlbumAssigned)
	require.False(t, result.VariousAssigned)
}

func TestGroupingRuntimeFailureRejectsBothAssignments(t *testing.T) {
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"literal","value":true},"then":[{"set":"group.various_artists","value":{"op":"literal","value":true}},{"set":"group.album","value":{"op":"field","field":"raw:/label"}}]}]}`), &program))
	programs, err := CompileGroupingPrograms([]IdentityFilter{{ID: "invalid", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &program}})
	require.NoError(t, err)
	env := AlbumGroupingEnvironment(1, "Edition", false)
	env["raw:/label"] = filters.Integer(7)
	result, issues, err := programs.ResolveValues(context.Background(), env)
	require.NoError(t, err)
	require.NotEmpty(t, issues)
	require.False(t, result.AlbumAssigned)
	require.False(t, result.VariousAssigned)
	require.False(t, result.VariousArtists)
}

package sources

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamedSpotifySourcesKeepIdentityWithOneServiceChoice(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for index, label := range []string{"", "parent-one", "parent-two"} {
		body := strings.Replace(complete, "03:04:05", []string{"03:04:05", "03:04:06", "03:04:07"}[index], 1)
		_, err := s.ImportSpotify(ctx, bytes.NewBufferString("["+body+"]"), "history.json", label, 1)
		require.NoError(t, err)
	}
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	require.Len(t, state.Sources, 3)
	require.Equal(t, "parent-one", state.Sources[1].Label)
	require.Equal(t, "parent-two", state.Sources[2].Label)
	choices := state.Blocks.Schemas["attribution:direct"].Choices["source.service"]
	require.Len(t, choices, 1)
	require.Equal(t, "spotify", choices[0].Value)
}

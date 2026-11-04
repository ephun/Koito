package sources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeCatalogInputsMatchRebuildInputs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	tx, err := s.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	items, err := catalogNameItems(ctx, tx)
	require.NoError(t, err)
	identifiers, err := catalogIdentifierItems(ctx, tx)
	require.NoError(t, err)
	require.NotEmpty(t, items)
	for _, item := range items {
		native, identifier, err := nativeCatalogItem(ctx, tx, item.kind, item.id)
		require.NoError(t, err)
		require.Equal(t, item, native)
		require.Contains(t, identifiers, identifier)
	}
}

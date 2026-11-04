package sqlite

import (
	"context"
	"database/sql"

	"github.com/gabehf/koito/internal/music"
	"github.com/gabehf/koito/internal/sources"
)

func refreshNativeCatalogItem(ctx context.Context, tx *sql.Tx, kind string, id int64) error {
	if err := sources.RefreshNativeCatalog(ctx, tx, kind, id); err != nil {
		return err
	}
	switch kind {
	case "track":
		sensitive, err := music.CaseSensitive(ctx, tx)
		if err != nil {
			return err
		}
		if err = music.AssignHistory(ctx, tx, id, sensitive); err != nil {
			return err
		}
		return music.RefreshScope(ctx, tx, id)
	case "album":
		return music.AssignAlbumGroup(ctx, tx, id)
	}
	return nil
}

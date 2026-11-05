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
	var config string
	if err := tx.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&config); err != nil {
		return err
	}
	if err := music.AssignCatalogMatches(ctx, tx, config, kind, id); err != nil {
		return err
	}
	if kind == "artist" {
		if err := sources.RefreshMatchedCatalog(ctx, tx); err != nil {
			return err
		}
		sensitive, err := music.CaseSensitive(ctx, tx)
		if err != nil {
			return err
		}
		if err = music.RefreshHistories(ctx, tx, sensitive, config); err != nil {
			return err
		}
		return music.RefreshScope(ctx, tx, 0, config)
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

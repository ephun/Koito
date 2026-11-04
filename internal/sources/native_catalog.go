package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

// RefreshNativeCatalog evaluates saved catalog corrections inside the capture
// transaction. Shared aliases also refresh affected artists' source genres.
func RefreshNativeCatalog(ctx context.Context, tx *sql.Tx, kind string, id int64) error {
	version, err := readRuleVersion(ctx, tx, 0)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if kind == "artist" {
		// A new shared alias can make another artist's source genres ambiguous.
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT a.artist_id FROM artist_aliases a JOIN artist_aliases current ON current.alias=a.alias WHERE current.artist_id=? ORDER BY a.artist_id`, id)
		if err != nil {
			return err
		}
		ids := []int64{}
		for rows.Next() {
			var artist int64
			if err = rows.Scan(&artist); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, artist)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, artist := range ids {
			if err = publishArtistGenres(ctx, tx, version.Config, artist); err != nil {
				return err
			}
			if artist != id {
				item, _, err := nativeCatalogItem(ctx, tx, "artist", artist)
				if err != nil {
					return err
				}
				if err = publishCatalogNames(ctx, tx, version.Config, item); err != nil {
					return err
				}
			}
		}
	}
	active := false
	for _, rule := range version.Config.Rules {
		_, genres := namingTargets(rule.Program)
		if rule.Enabled && rule.Program != nil && (statementsRead(rule.Program.Steps, "catalog.kind") || genres && kind == "artist") && (rule.Type == "presentation" && rule.Subtype == "naming" || rule.Type == "matching" && rule.Subtype == "music") {
			active = true
		}
	}
	if !active {
		return nil
	}
	item, identifier, err := nativeCatalogItem(ctx, tx, kind, id)
	if err != nil {
		return err
	}
	if err = publishCatalogIdentifiers(ctx, tx, version.Config, identifier); err != nil {
		return err
	}
	return publishCatalogNames(ctx, tx, version.Config, item)
}

func nativeCatalogItem(ctx context.Context, tx *sql.Tx, kind string, id int64) (catalogNameItem, catalogIdentifierItem, error) {
	item := catalogNameItem{kind: kind, id: id, metadata: filters.Env{}}
	identifier := catalogIdentifierItem{kind: kind, id: id}
	var nameQuery, aliasQuery, creditQuery string
	switch kind {
	case "artist":
		nameQuery = `SELECT n.name,0,COALESCE(a.musicbrainz_id,'') FROM artists a JOIN artists_with_name n ON n.id=a.id WHERE a.id=?`
		aliasQuery = `SELECT alias FROM artist_aliases WHERE artist_id=? ORDER BY alias`
		creditQuery = `SELECT id,name,0 FROM artists_with_name WHERE id=?`
	case "album":
		nameQuery = `SELECT n.title,a.id,COALESCE(a.musicbrainz_id,'') FROM releases a JOIN releases_with_title n ON n.id=a.id WHERE a.id=?`
		aliasQuery = `SELECT alias FROM release_aliases WHERE release_id=? ORDER BY alias`
		creditQuery = `SELECT a.id,a.name,c.is_primary FROM artist_releases c JOIN artists_with_name a ON a.id=c.artist_id WHERE c.release_id=? ORDER BY c.is_primary DESC,a.id`
	case "track":
		nameQuery = `SELECT n.title,a.release_id,COALESCE(a.musicbrainz_id,(SELECT r.musicbrainz_id FROM track_recordings tr JOIN recordings r ON r.recording_key=tr.recording_key WHERE tr.track_id=a.id),'') FROM tracks a JOIN tracks_with_title n ON n.id=a.id WHERE a.id=?`
		aliasQuery = `SELECT alias FROM track_aliases WHERE track_id=? ORDER BY alias`
		creditQuery = `SELECT a.id,a.name,c.is_primary FROM artist_tracks c JOIN artists_with_name a ON a.id=c.artist_id WHERE c.track_id=? ORDER BY c.is_primary DESC,a.id`
	default:
		return item, identifier, fmt.Errorf("Unknown catalog context.")
	}
	if err := tx.QueryRowContext(ctx, nameQuery, id).Scan(&item.name, &item.albumID, &identifier.original); err != nil {
		return item, identifier, err
	}
	rows, err := tx.QueryContext(ctx, aliasQuery, id)
	if err != nil {
		return item, identifier, err
	}
	aliases := []filters.Value{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return item, identifier, err
		}
		aliases = append(aliases, filters.String(name))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return item, identifier, err
	}
	item.metadata["original.aliases"] = filters.Values(aliases...)
	rows, err = tx.QueryContext(ctx, creditQuery, id)
	if err != nil {
		return item, identifier, err
	}
	var names, ids, primary []filters.Value
	for rows.Next() {
		var artist int64
		var name string
		var isPrimary bool
		if err = rows.Scan(&artist, &name, &isPrimary); err != nil {
			rows.Close()
			return item, identifier, err
		}
		names = append(names, filters.String(name))
		ids = append(ids, filters.Integer(artist))
		if isPrimary {
			primary = append(primary, filters.Integer(artist))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return item, identifier, err
	}
	item.metadata["original.artists"] = filters.Values(names...)
	item.metadata["catalog.artist_ids"] = filters.Values(ids...)
	item.metadata["original.primary_artist_ids"] = filters.Values(primary...)
	if len(names) == 1 {
		item.metadata["original.artist"] = names[0]
	}
	if kind == "track" {
		var album string
		if err = tx.QueryRowContext(ctx, `SELECT title FROM releases_with_title WHERE id=?`, item.albumID).Scan(&album); err != nil {
			return item, identifier, err
		}
		item.metadata["original.album"] = filters.String(album)
	}
	rows, err = tx.QueryContext(ctx, `SELECT id FROM artists_with_name ORDER BY id`)
	if err != nil {
		return item, identifier, err
	}
	available := []filters.Value{}
	for rows.Next() {
		var artist int64
		if err = rows.Scan(&artist); err != nil {
			rows.Close()
			return item, identifier, err
		}
		available = append(available, filters.Integer(artist))
	}
	err = rows.Err()
	rows.Close()
	item.metadata["catalog.available_artist_ids"] = filters.Values(available...)
	return item, identifier, err
}

-- +goose Up
-- Keep primary-name joins selective as playlist imports expand the catalog.
-- SQLite can otherwise choose an automatic index on is_primary alone.
CREATE INDEX idx_track_aliases_primary_track ON track_aliases(is_primary,track_id,alias);
CREATE INDEX idx_release_aliases_primary_release ON release_aliases(is_primary,release_id,alias);
CREATE INDEX idx_artist_aliases_primary_artist ON artist_aliases(is_primary,artist_id,alias);

-- +goose Down
DROP INDEX idx_artist_aliases_primary_artist;
DROP INDEX idx_release_aliases_primary_release;
DROP INDEX idx_track_aliases_primary_track;

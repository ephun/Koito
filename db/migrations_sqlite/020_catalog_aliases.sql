-- +goose Up
CREATE TABLE catalog_alias_sets (
 kind TEXT NOT NULL CHECK(kind IN ('artist','album','track')),
 item_id INTEGER NOT NULL,
 PRIMARY KEY(kind,item_id)
);
CREATE TABLE catalog_aliases (
 kind TEXT NOT NULL,
 item_id INTEGER NOT NULL,
 alias TEXT NOT NULL CHECK(length(trim(alias))>0),
 PRIMARY KEY(kind,item_id,alias),
 FOREIGN KEY(kind,item_id) REFERENCES catalog_alias_sets(kind,item_id) ON DELETE CASCADE
);
CREATE INDEX catalog_aliases_name ON catalog_aliases(kind,alias,item_id);
CREATE VIEW displayed_artist_aliases AS
 SELECT artist_id,alias,source,is_primary FROM artist_aliases a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_alias_sets c WHERE c.kind='artist' AND c.item_id=a.artist_id)
 UNION ALL SELECT item_id,alias,'Filter',0 FROM catalog_aliases WHERE kind='artist';
CREATE VIEW displayed_release_aliases AS
 SELECT release_id,alias,source,is_primary FROM release_aliases a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_alias_sets c WHERE c.kind='album' AND c.item_id=a.release_id)
 UNION ALL SELECT item_id,alias,'Filter',0 FROM catalog_aliases WHERE kind='album';
CREATE VIEW displayed_track_aliases AS
 SELECT track_id,alias,source,is_primary FROM track_aliases a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_alias_sets c WHERE c.kind='track' AND c.item_id=a.track_id)
 UNION ALL SELECT item_id,alias,'Filter',0 FROM catalog_aliases WHERE kind='track';
-- +goose Down
DROP VIEW displayed_track_aliases;
DROP VIEW displayed_release_aliases;
DROP VIEW displayed_artist_aliases;
DROP TABLE catalog_aliases;
DROP TABLE catalog_alias_sets;

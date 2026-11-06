-- +goose Up
CREATE TABLE catalog_credit_sets (
 kind TEXT NOT NULL CHECK(kind IN ('album','track')),
 item_id INTEGER NOT NULL,
 PRIMARY KEY(kind,item_id)
);
CREATE TABLE catalog_credits (
 kind TEXT NOT NULL,
 item_id INTEGER NOT NULL,
 artist_id INTEGER NOT NULL REFERENCES artists(id),
 is_primary INTEGER NOT NULL CHECK(is_primary IN (0,1)),
 position INTEGER NOT NULL,
 PRIMARY KEY(kind,item_id,artist_id),
 FOREIGN KEY(kind,item_id) REFERENCES catalog_credit_sets(kind,item_id) ON DELETE CASCADE
);
CREATE INDEX catalog_credits_artist ON catalog_credits(artist_id,kind,item_id);
CREATE VIEW displayed_artist_tracks AS
 SELECT artist_id,track_id,is_primary FROM artist_tracks a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_credit_sets c WHERE c.kind='track' AND c.item_id=a.track_id)
 UNION ALL SELECT artist_id,item_id,is_primary FROM catalog_credits WHERE kind='track';
CREATE VIEW displayed_artist_releases AS
 SELECT artist_id,release_id,is_primary FROM artist_releases a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_credit_sets c WHERE c.kind='album' AND c.item_id=a.release_id)
 UNION ALL SELECT artist_id,item_id,is_primary FROM catalog_credits WHERE kind='album';
-- +goose Down
DROP VIEW displayed_artist_releases;
DROP VIEW displayed_artist_tracks;
DROP TABLE catalog_credits;
DROP TABLE catalog_credit_sets;

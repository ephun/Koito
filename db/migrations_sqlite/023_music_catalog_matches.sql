-- +goose Up
CREATE TABLE music_artist_matches (
 artist_id INTEGER PRIMARY KEY REFERENCES artists(id) ON DELETE CASCADE,
 matched_artist_id INTEGER NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
 comparison_key TEXT NOT NULL
);
CREATE INDEX music_artist_matches_key ON music_artist_matches(comparison_key);
CREATE INDEX music_artist_matches_target ON music_artist_matches(matched_artist_id);
CREATE TABLE music_track_artists (
 track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
 original_artist_id INTEGER NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
 artist_id INTEGER NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
 PRIMARY KEY(track_id,original_artist_id)
);
CREATE TABLE track_catalog_comparisons (
 track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
 artist TEXT NOT NULL,
 album TEXT NOT NULL
);
CREATE TABLE music_release_groups (
 track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
 group_id INTEGER NOT NULL REFERENCES album_groups(id) ON DELETE CASCADE
);
CREATE INDEX music_release_groups_group ON music_release_groups(group_id);
CREATE TABLE music_album_comparisons (
 track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
 release_id INTEGER NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
 comparison_key TEXT NOT NULL,
 artist TEXT NOT NULL,
 artist_credits TEXT NOT NULL,
 identifier TEXT NOT NULL
);
CREATE INDEX music_album_comparisons_key ON music_album_comparisons(comparison_key);
CREATE INDEX music_album_groups_key ON album_groups(json_extract(group_key,'$[0]'),json_extract(group_key,'$[1]'));
CREATE TABLE native_track_variants (
 original_track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
 track_id INTEGER NOT NULL UNIQUE REFERENCES tracks(id) ON DELETE CASCADE
);
CREATE VIEW catalog_track_origins AS
 SELECT t.id AS track_id,COALESCE(n.original_track_id,(SELECT MIN(v.base_track_id) FROM source_history_variants v WHERE v.track_id=t.id),t.id) AS original_track_id
 FROM tracks t LEFT JOIN native_track_variants n ON n.track_id=t.id;
CREATE TABLE music_history_canonicals (
 music_track_id INTEGER PRIMARY KEY REFERENCES music_tracks(id) ON DELETE CASCADE,
 track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE
);
DROP VIEW track_history_members;
CREATE VIEW track_history_members AS
 SELECT h.track_id,h.music_track_id,COALESCE(c.track_id,(SELECT MIN(m.track_id) FROM track_histories m WHERE m.music_track_id=h.music_track_id)) AS canonical_track_id
 FROM track_histories h LEFT JOIN music_history_canonicals c ON c.music_track_id=h.music_track_id
 UNION ALL SELECT t.id,NULL,t.id FROM tracks t WHERE NOT EXISTS(SELECT 1 FROM track_histories h WHERE h.track_id=t.id);
CREATE VIEW catalog_artist_tracks AS
 SELECT artist_id,track_id,is_primary FROM artist_tracks a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_credit_sets c WHERE c.kind='track' AND c.item_id=a.track_id)
 UNION ALL SELECT artist_id,item_id,is_primary FROM catalog_credits WHERE kind='track';
CREATE VIEW catalog_artist_releases AS
 SELECT artist_id,release_id,is_primary FROM artist_releases a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_credit_sets c WHERE c.kind='album' AND c.item_id=a.release_id)
 UNION ALL SELECT artist_id,item_id,is_primary FROM catalog_credits WHERE kind='album';
DROP VIEW displayed_artist_tracks;
DROP VIEW displayed_artist_releases;
CREATE VIEW displayed_artist_tracks AS
 SELECT COALESCE(t.artist_id,m.matched_artist_id,a.artist_id) AS artist_id,a.track_id,MAX(a.is_primary) AS is_primary
 FROM catalog_artist_tracks a
 LEFT JOIN music_track_artists t ON t.track_id=a.track_id AND t.original_artist_id=a.artist_id
 LEFT JOIN music_artist_matches m ON m.artist_id=a.artist_id
 GROUP BY a.track_id,COALESCE(t.artist_id,m.matched_artist_id,a.artist_id);
CREATE VIEW displayed_artist_releases AS
 SELECT COALESCE(m.matched_artist_id,a.artist_id) AS artist_id,a.release_id,MAX(a.is_primary) AS is_primary
 FROM catalog_artist_releases a LEFT JOIN music_artist_matches m ON m.artist_id=a.artist_id
 GROUP BY a.release_id,COALESCE(m.matched_artist_id,a.artist_id);
CREATE VIEW displayed_artist_genres AS
 SELECT g.artist_id,g.genre FROM artist_genres g LEFT JOIN music_artist_matches m ON m.artist_id=g.artist_id
 WHERE COALESCE(m.matched_artist_id,g.artist_id)=g.artist_id;
DROP VIEW album_report_counts;
DROP VIEW album_report_events;
CREATE VIEW album_report_events AS
 SELECT l.*,t.release_id,COALESCE(s.group_id,n.group_id,m.group_id,0) AS album_group_id
 FROM displayed_listens l JOIN tracks t ON t.id=l.track_id
 LEFT JOIN source_play_groupings s ON s.play_key=l.play_key
 LEFT JOIN native_release_groups n ON n.release_id=t.release_id
 LEFT JOIN music_release_groups m ON m.track_id=l.track_id;
CREATE VIEW album_report_counts AS
 SELECT track_id,user_id,listened_at,release_id,album_group_id,1 AS amount FROM album_report_events
 UNION ALL
 SELECT a.track_id,a.user_id,NULL,t.release_id,COALESCE(s.group_id,n.group_id,m.group_id,0),a.undated_count
 FROM source_aggregates a JOIN tracks t ON t.id=a.track_id
 LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=a.aggregate_key
 LEFT JOIN native_release_groups n ON n.release_id=t.release_id
 LEFT JOIN music_release_groups m ON m.track_id=a.track_id
 WHERE a.undated_count>0 AND NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=a.track_id AND s.included=0);

-- +goose Down
DROP VIEW track_history_members;
CREATE VIEW track_history_members AS
 SELECT h.track_id,h.music_track_id,(SELECT MIN(m.track_id) FROM track_histories m WHERE m.music_track_id=h.music_track_id) AS canonical_track_id
 FROM track_histories h UNION ALL SELECT t.id,NULL,t.id FROM tracks t WHERE NOT EXISTS(SELECT 1 FROM track_histories h WHERE h.track_id=t.id);
DROP TABLE music_history_canonicals;
DROP VIEW album_report_counts;
DROP VIEW album_report_events;
CREATE VIEW album_report_events AS
 SELECT l.*,t.release_id,COALESCE(s.group_id,n.group_id,0) AS album_group_id
 FROM displayed_listens l JOIN tracks t ON t.id=l.track_id
 LEFT JOIN source_play_groupings s ON s.play_key=l.play_key
 LEFT JOIN native_release_groups n ON n.release_id=t.release_id;
CREATE VIEW album_report_counts AS
 SELECT track_id,user_id,listened_at,release_id,album_group_id,1 AS amount FROM album_report_events
 UNION ALL
 SELECT a.track_id,a.user_id,NULL,t.release_id,COALESCE(s.group_id,n.group_id,0),a.undated_count
 FROM source_aggregates a JOIN tracks t ON t.id=a.track_id
 LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=a.aggregate_key
 LEFT JOIN native_release_groups n ON n.release_id=t.release_id
 WHERE a.undated_count>0 AND NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=a.track_id AND s.included=0);
DROP VIEW displayed_artist_genres;
DROP VIEW displayed_artist_tracks;
DROP VIEW displayed_artist_releases;
CREATE VIEW displayed_artist_tracks AS
 SELECT artist_id,track_id,is_primary FROM artist_tracks a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_credit_sets c WHERE c.kind='track' AND c.item_id=a.track_id)
 UNION ALL SELECT artist_id,item_id,is_primary FROM catalog_credits WHERE kind='track';
CREATE VIEW displayed_artist_releases AS
 SELECT artist_id,release_id,is_primary FROM artist_releases a
 WHERE NOT EXISTS(SELECT 1 FROM catalog_credit_sets c WHERE c.kind='album' AND c.item_id=a.release_id)
 UNION ALL SELECT artist_id,item_id,is_primary FROM catalog_credits WHERE kind='album';
DROP VIEW catalog_artist_tracks;
DROP VIEW catalog_artist_releases;
DROP TABLE music_release_groups;
DROP VIEW catalog_track_origins;
DROP TABLE native_track_variants;
DROP INDEX music_album_groups_key;
DROP TABLE music_album_comparisons;
DROP TABLE track_catalog_comparisons;
DROP TABLE music_track_artists;
DROP TABLE music_artist_matches;

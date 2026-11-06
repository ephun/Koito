-- +goose Up
CREATE TABLE aggregate_scope (
 aggregate_key TEXT PRIMARY KEY REFERENCES source_aggregates(aggregate_key) ON DELETE CASCADE,
 track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
 included INTEGER NOT NULL CHECK(included IN (0,1)),
 issues_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(issues_json))
);
CREATE INDEX aggregate_scope_track ON aggregate_scope(track_id);
CREATE VIEW displayed_aggregates AS SELECT a.* FROM source_aggregates a
WHERE COALESCE((SELECT s.included FROM aggregate_scope s WHERE s.aggregate_key=a.aggregate_key),
 (SELECT s.included FROM track_scope s WHERE s.track_id=a.track_id),1)=1;
DROP VIEW listening_counts;
CREATE VIEW listening_counts AS
SELECT track_id,user_id,listened_at,1 AS amount FROM displayed_listens
UNION ALL
SELECT track_id,user_id,NULL,SUM(undated_count) FROM displayed_aggregates
WHERE undated_count>0 AND track_id IS NOT NULL GROUP BY track_id,user_id;
DROP VIEW album_report_counts;
CREATE VIEW album_report_counts AS
SELECT track_id,user_id,listened_at,release_id,album_group_id,1 AS amount FROM album_report_events
UNION ALL
SELECT a.track_id,a.user_id,NULL,t.release_id,COALESCE(s.group_id,n.group_id,m.group_id,0),a.undated_count
FROM displayed_aggregates a JOIN tracks t ON t.id=a.track_id
LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=a.aggregate_key
LEFT JOIN native_release_groups n ON n.release_id=t.release_id
LEFT JOIN music_release_groups m ON m.track_id=a.track_id WHERE a.undated_count>0;

-- +goose Down
DROP VIEW album_report_counts;
CREATE VIEW album_report_counts AS
SELECT track_id,user_id,listened_at,release_id,album_group_id,1 AS amount FROM album_report_events
UNION ALL
SELECT a.track_id,a.user_id,NULL,t.release_id,COALESCE(s.group_id,n.group_id,m.group_id,0),a.undated_count
FROM source_aggregates a JOIN tracks t ON t.id=a.track_id
LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=a.aggregate_key
LEFT JOIN native_release_groups n ON n.release_id=t.release_id
LEFT JOIN music_release_groups m ON m.track_id=a.track_id
WHERE a.undated_count>0 AND NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=a.track_id AND s.included=0);
DROP VIEW listening_counts;
CREATE VIEW listening_counts AS
SELECT track_id,user_id,listened_at,1 AS amount FROM displayed_listens
UNION ALL
SELECT track_id,user_id,NULL,SUM(undated_count) FROM source_aggregates
WHERE undated_count>0 AND track_id IS NOT NULL
AND NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=source_aggregates.track_id AND s.included=0)
GROUP BY track_id,user_id;
DROP VIEW displayed_aggregates;
DROP TABLE aggregate_scope;

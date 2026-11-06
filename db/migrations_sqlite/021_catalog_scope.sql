-- +goose Up
CREATE TABLE track_scope (
 track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
 included INTEGER NOT NULL CHECK(included IN (0,1))
);
CREATE TABLE track_scope_issues (
 track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
 issues_json TEXT NOT NULL CHECK(json_valid(issues_json))
);
CREATE VIEW displayed_listens AS SELECT l.* FROM listens l
WHERE NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=l.track_id AND s.included=0);
DROP VIEW album_report_counts;
DROP VIEW album_report_events;
DROP VIEW listening_counts;
CREATE VIEW listening_counts AS
SELECT track_id,user_id,listened_at,1 AS amount FROM displayed_listens
UNION ALL
SELECT track_id,user_id,NULL AS listened_at,SUM(undated_count) AS amount
FROM source_aggregates WHERE NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=source_aggregates.track_id AND s.included=0) AND undated_count>0 AND track_id IS NOT NULL
GROUP BY track_id,user_id;
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
-- +goose Down
DROP VIEW album_report_counts;
DROP VIEW album_report_events;
DROP VIEW listening_counts;
CREATE VIEW listening_counts AS
SELECT track_id,user_id,listened_at,1 AS amount FROM listens
UNION ALL
SELECT track_id,user_id,NULL AS listened_at,SUM(undated_count) AS amount
FROM source_aggregates WHERE undated_count>0 AND track_id IS NOT NULL
GROUP BY track_id,user_id;
CREATE VIEW album_report_events AS
SELECT l.*,t.release_id,COALESCE(s.group_id,n.group_id,0) AS album_group_id
FROM listens l JOIN tracks t ON t.id=l.track_id
LEFT JOIN source_play_groupings s ON s.play_key=l.play_key
LEFT JOIN native_release_groups n ON n.release_id=t.release_id;
CREATE VIEW album_report_counts AS
SELECT track_id,user_id,listened_at,release_id,album_group_id,1 AS amount FROM album_report_events
UNION ALL
SELECT a.track_id,a.user_id,NULL,t.release_id,COALESCE(s.group_id,n.group_id,0),a.undated_count
FROM source_aggregates a JOIN tracks t ON t.id=a.track_id
LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=a.aggregate_key
LEFT JOIN native_release_groups n ON n.release_id=t.release_id
WHERE a.undated_count>0;
DROP VIEW displayed_listens;
DROP TABLE track_scope_issues;
DROP TABLE track_scope;

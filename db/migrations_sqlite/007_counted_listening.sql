-- +goose Up
-- Undated amounts have no calendar timestamp. Weighted rows let all-time
-- totals include them without expanding counts into invented listening events.
CREATE VIEW listening_counts AS
SELECT track_id,user_id,listened_at,1 AS amount FROM listens
UNION ALL
SELECT track_id,user_id,NULL AS listened_at,SUM(undated_count) AS amount
FROM source_aggregates WHERE undated_count>0 AND track_id IS NOT NULL
GROUP BY track_id,user_id;

-- +goose Down
-- Keep the existing backup requirement when reverting inference support.
-- +goose StatementBegin
CREATE TEMP TABLE koito_counts_rollback_guard (id INTEGER);
CREATE TEMP TRIGGER koito_counts_rollback_requires_backup BEFORE INSERT ON koito_counts_rollback_guard BEGIN
    SELECT RAISE(ABORT,'Restore a pre-migration backup to remove undated counts.');
END;
INSERT INTO koito_counts_rollback_guard VALUES(1);
-- +goose StatementEnd

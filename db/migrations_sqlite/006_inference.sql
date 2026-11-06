-- +goose Up
CREATE TABLE source_play_details (
    play_key TEXT PRIMARY KEY,
    evidence_level TEXT NOT NULL CHECK(evidence_level IN ('observed','last_played','estimated')),
    interval_start TEXT,
    interval_end TEXT,
    attribution_json TEXT NOT NULL CHECK(json_valid(attribution_json)),
    version_id INTEGER NOT NULL REFERENCES filter_versions(id)
);
CREATE TABLE source_play_records (
    play_key TEXT NOT NULL REFERENCES source_play_details(play_key) ON DELETE CASCADE,
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    PRIMARY KEY(play_key,record_id)
);
CREATE INDEX source_play_records_record ON source_play_records(record_id,play_key);
CREATE TABLE source_aggregates (
    aggregate_key TEXT PRIMARY KEY,
    library_key TEXT NOT NULL,
    track_key TEXT NOT NULL,
    representative_record INTEGER NOT NULL REFERENCES source_records(id),
    track_id INTEGER REFERENCES tracks(id) ON DELETE SET NULL,
    user_id INTEGER NOT NULL REFERENCES users(id),
    available_count INTEGER NOT NULL CHECK(available_count>=0),
    attributed_count INTEGER NOT NULL CHECK(attributed_count>=0 AND attributed_count<=available_count),
    observed_count INTEGER NOT NULL CHECK(observed_count>=0),
    overlap_count INTEGER NOT NULL CHECK(overlap_count>=0),
    last_played_count INTEGER NOT NULL CHECK(last_played_count>=0),
    estimated_count INTEGER NOT NULL CHECK(estimated_count>=0),
    undated_count INTEGER NOT NULL CHECK(undated_count>=0),
    details_json TEXT NOT NULL CHECK(json_valid(details_json)),
    version_id INTEGER NOT NULL REFERENCES filter_versions(id),
    CHECK(observed_count+overlap_count+last_played_count+estimated_count+undated_count=attributed_count)
);
CREATE INDEX source_aggregates_track ON source_aggregates(track_id);
CREATE TABLE source_aggregate_records (
    aggregate_key TEXT NOT NULL REFERENCES source_aggregates(aggregate_key) ON DELETE CASCADE,
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    PRIMARY KEY(aggregate_key,record_id)
);
CREATE INDEX source_aggregate_records_record ON source_aggregate_records(record_id,aggregate_key);

-- +goose Down
-- Older binaries cannot distinguish estimated events or attributed counts.
-- Preserve the existing rollback requirement instead of discarding that context.
-- +goose StatementBegin
CREATE TEMP TABLE koito_inference_rollback_guard (id INTEGER);
CREATE TEMP TRIGGER koito_inference_rollback_requires_backup BEFORE INSERT ON koito_inference_rollback_guard BEGIN
    SELECT RAISE(ABORT,'Restore a pre-migration backup to remove inference evidence.');
END;
INSERT INTO koito_inference_rollback_guard VALUES(1);
-- +goose StatementEnd

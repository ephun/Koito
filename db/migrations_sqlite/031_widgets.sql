-- +goose Up
CREATE TABLE widgets (
 id TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL REFERENCES users(id),
 version INTEGER NOT NULL,
 definition TEXT NOT NULL CHECK(json_valid(definition)),
 updated_at INTEGER NOT NULL
);
CREATE TABLE widget_data_revision (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL);
INSERT INTO widget_data_revision VALUES(1,1);
CREATE TABLE widget_query_cache (
 cache_key TEXT PRIMARY KEY,
 revision INTEGER NOT NULL,
 result TEXT NOT NULL CHECK(json_valid(result)),
 created_at INTEGER NOT NULL
);
CREATE INDEX widget_cache_revision ON widget_query_cache(revision);
-- +goose StatementBegin
CREATE TRIGGER widget_revision_listens_insert AFTER INSERT ON listens BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_listens_update AFTER UPDATE ON listens BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_listens_delete AFTER DELETE ON listens BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_projection_runs_insert AFTER INSERT ON source_projection_runs BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_projection_runs_update AFTER UPDATE ON source_projection_runs BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_projection_runs_delete AFTER DELETE ON source_projection_runs BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregates_insert AFTER INSERT ON source_aggregates BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregates_update AFTER UPDATE ON source_aggregates BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregates_delete AFTER DELETE ON source_aggregates BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_records_insert AFTER INSERT ON source_play_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_records_update AFTER UPDATE ON source_play_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_records_delete AFTER DELETE ON source_play_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregate_records_insert AFTER INSERT ON source_aggregate_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregate_records_update AFTER UPDATE ON source_aggregate_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregate_records_delete AFTER DELETE ON source_aggregate_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_details_insert AFTER INSERT ON source_play_details BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_details_update AFTER UPDATE ON source_play_details BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_details_delete AFTER DELETE ON source_play_details BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_records_insert AFTER INSERT ON source_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_records_update AFTER UPDATE ON source_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_records_delete AFTER DELETE ON source_records BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_accounts_insert AFTER INSERT ON source_accounts BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_accounts_update AFTER UPDATE ON source_accounts BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_accounts_delete AFTER DELETE ON source_accounts BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_groupings_insert AFTER INSERT ON source_play_groupings BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_groupings_update AFTER UPDATE ON source_play_groupings BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_play_groupings_delete AFTER DELETE ON source_play_groupings BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregate_groupings_insert AFTER INSERT ON source_aggregate_groupings BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregate_groupings_update AFTER UPDATE ON source_aggregate_groupings BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_source_aggregate_groupings_delete AFTER DELETE ON source_aggregate_groupings BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_native_release_groups_insert AFTER INSERT ON native_release_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_native_release_groups_update AFTER UPDATE ON native_release_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_native_release_groups_delete AFTER DELETE ON native_release_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_release_groups_insert AFTER INSERT ON music_release_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_release_groups_update AFTER UPDATE ON music_release_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_release_groups_delete AFTER DELETE ON music_release_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_album_groups_insert AFTER INSERT ON album_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_album_groups_update AFTER UPDATE ON album_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_album_groups_delete AFTER DELETE ON album_groups BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_names_insert AFTER INSERT ON catalog_names BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_names_update AFTER UPDATE ON catalog_names BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_names_delete AFTER DELETE ON catalog_names BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_tracks_insert AFTER INSERT ON tracks BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_tracks_update AFTER UPDATE ON tracks BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_tracks_delete AFTER DELETE ON tracks BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_aliases_insert AFTER INSERT ON track_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_aliases_update AFTER UPDATE ON track_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_aliases_delete AFTER DELETE ON track_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_release_aliases_insert AFTER INSERT ON release_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_release_aliases_update AFTER UPDATE ON release_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_release_aliases_delete AFTER DELETE ON release_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_artist_aliases_insert AFTER INSERT ON artist_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_artist_aliases_update AFTER UPDATE ON artist_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_artist_aliases_delete AFTER DELETE ON artist_aliases BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_artist_tracks_insert AFTER INSERT ON artist_tracks BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_artist_tracks_update AFTER UPDATE ON artist_tracks BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_artist_tracks_delete AFTER DELETE ON artist_tracks BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_credits_insert AFTER INSERT ON catalog_credits BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_credits_update AFTER UPDATE ON catalog_credits BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_credits_delete AFTER DELETE ON catalog_credits BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_credit_sets_insert AFTER INSERT ON catalog_credit_sets BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_credit_sets_update AFTER UPDATE ON catalog_credit_sets BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_catalog_credit_sets_delete AFTER DELETE ON catalog_credit_sets BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_artist_matches_insert AFTER INSERT ON music_artist_matches BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_artist_matches_update AFTER UPDATE ON music_artist_matches BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_artist_matches_delete AFTER DELETE ON music_artist_matches BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_track_artists_insert AFTER INSERT ON music_track_artists BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_track_artists_update AFTER UPDATE ON music_track_artists BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_track_artists_delete AFTER DELETE ON music_track_artists BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_histories_insert AFTER INSERT ON track_histories BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_histories_update AFTER UPDATE ON track_histories BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_histories_delete AFTER DELETE ON track_histories BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_history_canonicals_insert AFTER INSERT ON music_history_canonicals BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_history_canonicals_update AFTER UPDATE ON music_history_canonicals BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_music_history_canonicals_delete AFTER DELETE ON music_history_canonicals BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_listen_scope_insert AFTER INSERT ON listen_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_listen_scope_update AFTER UPDATE ON listen_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_listen_scope_delete AFTER DELETE ON listen_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_scope_insert AFTER INSERT ON track_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_scope_update AFTER UPDATE ON track_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_track_scope_delete AFTER DELETE ON track_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_aggregate_scope_insert AFTER INSERT ON aggregate_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_aggregate_scope_update AFTER UPDATE ON aggregate_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER widget_revision_aggregate_scope_delete AFTER DELETE ON aggregate_scope BEGIN
 UPDATE widget_data_revision SET revision=revision+1 WHERE id=1;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER widget_revision_listens_insert;
DROP TRIGGER widget_revision_listens_update;
DROP TRIGGER widget_revision_listens_delete;
DROP TRIGGER widget_revision_source_projection_runs_insert;
DROP TRIGGER widget_revision_source_projection_runs_update;
DROP TRIGGER widget_revision_source_projection_runs_delete;
DROP TRIGGER widget_revision_source_aggregates_insert;
DROP TRIGGER widget_revision_source_aggregates_update;
DROP TRIGGER widget_revision_source_aggregates_delete;
DROP TRIGGER widget_revision_source_play_records_insert;
DROP TRIGGER widget_revision_source_play_records_update;
DROP TRIGGER widget_revision_source_play_records_delete;
DROP TRIGGER widget_revision_source_aggregate_records_insert;
DROP TRIGGER widget_revision_source_aggregate_records_update;
DROP TRIGGER widget_revision_source_aggregate_records_delete;
DROP TRIGGER widget_revision_source_play_details_insert;
DROP TRIGGER widget_revision_source_play_details_update;
DROP TRIGGER widget_revision_source_play_details_delete;
DROP TRIGGER widget_revision_source_records_insert;
DROP TRIGGER widget_revision_source_records_update;
DROP TRIGGER widget_revision_source_records_delete;
DROP TRIGGER widget_revision_source_accounts_insert;
DROP TRIGGER widget_revision_source_accounts_update;
DROP TRIGGER widget_revision_source_accounts_delete;
DROP TRIGGER widget_revision_source_play_groupings_insert;
DROP TRIGGER widget_revision_source_play_groupings_update;
DROP TRIGGER widget_revision_source_play_groupings_delete;
DROP TRIGGER widget_revision_source_aggregate_groupings_insert;
DROP TRIGGER widget_revision_source_aggregate_groupings_update;
DROP TRIGGER widget_revision_source_aggregate_groupings_delete;
DROP TRIGGER widget_revision_native_release_groups_insert;
DROP TRIGGER widget_revision_native_release_groups_update;
DROP TRIGGER widget_revision_native_release_groups_delete;
DROP TRIGGER widget_revision_music_release_groups_insert;
DROP TRIGGER widget_revision_music_release_groups_update;
DROP TRIGGER widget_revision_music_release_groups_delete;
DROP TRIGGER widget_revision_album_groups_insert;
DROP TRIGGER widget_revision_album_groups_update;
DROP TRIGGER widget_revision_album_groups_delete;
DROP TRIGGER widget_revision_catalog_names_insert;
DROP TRIGGER widget_revision_catalog_names_update;
DROP TRIGGER widget_revision_catalog_names_delete;
DROP TRIGGER widget_revision_tracks_insert;
DROP TRIGGER widget_revision_tracks_update;
DROP TRIGGER widget_revision_tracks_delete;
DROP TRIGGER widget_revision_track_aliases_insert;
DROP TRIGGER widget_revision_track_aliases_update;
DROP TRIGGER widget_revision_track_aliases_delete;
DROP TRIGGER widget_revision_release_aliases_insert;
DROP TRIGGER widget_revision_release_aliases_update;
DROP TRIGGER widget_revision_release_aliases_delete;
DROP TRIGGER widget_revision_artist_aliases_insert;
DROP TRIGGER widget_revision_artist_aliases_update;
DROP TRIGGER widget_revision_artist_aliases_delete;
DROP TRIGGER widget_revision_artist_tracks_insert;
DROP TRIGGER widget_revision_artist_tracks_update;
DROP TRIGGER widget_revision_artist_tracks_delete;
DROP TRIGGER widget_revision_catalog_credits_insert;
DROP TRIGGER widget_revision_catalog_credits_update;
DROP TRIGGER widget_revision_catalog_credits_delete;
DROP TRIGGER widget_revision_catalog_credit_sets_insert;
DROP TRIGGER widget_revision_catalog_credit_sets_update;
DROP TRIGGER widget_revision_catalog_credit_sets_delete;
DROP TRIGGER widget_revision_music_artist_matches_insert;
DROP TRIGGER widget_revision_music_artist_matches_update;
DROP TRIGGER widget_revision_music_artist_matches_delete;
DROP TRIGGER widget_revision_music_track_artists_insert;
DROP TRIGGER widget_revision_music_track_artists_update;
DROP TRIGGER widget_revision_music_track_artists_delete;
DROP TRIGGER widget_revision_track_histories_insert;
DROP TRIGGER widget_revision_track_histories_update;
DROP TRIGGER widget_revision_track_histories_delete;
DROP TRIGGER widget_revision_music_history_canonicals_insert;
DROP TRIGGER widget_revision_music_history_canonicals_update;
DROP TRIGGER widget_revision_music_history_canonicals_delete;
DROP TRIGGER widget_revision_listen_scope_insert;
DROP TRIGGER widget_revision_listen_scope_update;
DROP TRIGGER widget_revision_listen_scope_delete;
DROP TRIGGER widget_revision_track_scope_insert;
DROP TRIGGER widget_revision_track_scope_update;
DROP TRIGGER widget_revision_track_scope_delete;
DROP TRIGGER widget_revision_aggregate_scope_insert;
DROP TRIGGER widget_revision_aggregate_scope_update;
DROP TRIGGER widget_revision_aggregate_scope_delete;
DROP TABLE widget_query_cache;
DROP TABLE widget_data_revision;
DROP TABLE widgets;


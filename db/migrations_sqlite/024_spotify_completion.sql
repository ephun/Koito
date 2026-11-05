-- +goose Up
INSERT INTO filter_versions(parent_id,config_json,label,created_at)
SELECT v.id,json_insert(v.config_json,'$.rules[#]',json('{"id":"spotify-account-completion","name":"Spotify account history","type":"qualification","subtype":"completion","enabled":true,"scope":{},"settings":{},"program":{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.format"},{"op":"literal","value":"spotify-standard-v1"}]},"then":[{"set":"completion.count","value":{"op":"ge","args":[{"op":"field","field":"record.played_ms"},{"op":"literal","value":30000}]}}]}]}}')),
'Spotify account history completion',CAST(strftime('%s','now') AS INTEGER)
FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1;
UPDATE filter_state SET version_id=(SELECT MAX(id) FROM filter_versions) WHERE id=1;

-- +goose Down
-- Saved filter versions remain immutable. Revert through the filter manager.
SELECT 1;

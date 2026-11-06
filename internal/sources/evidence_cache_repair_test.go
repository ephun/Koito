package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/music"
	"github.com/stretchr/testify/require"
)

func TestUnnamedForwardingEvidenceDoesNotJoinUnrelatedMusic(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	input := `{"data":[{"uid":"source","componentId":3,"play":{"data":{"track":"Song","artists":["Artist"],"album":"Album","playDate":"2020-01-01T00:00:00Z"}}},{"uid":"delivery","componentId":1,"parentId":1,"parent":{"uid":"source"},"play":{}}],"meta":{"total":2,"limit":100,"offset":0}}`
	_, err := s.Import(ctx, strings.NewReader(input), "router.json", 1)
	require.NoError(t, err)
	saveTestRules(t, s, Rule{ID: "self", Type: "attribution", Subtype: "direct", Enabled: true, Program: assignWhen(literalExpr(true), "listener", literalExpr("self"))})
	require.Equal(t, 2, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "source_play_records"))
	var blank int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_personal_evidence_music WHERE music_key=?`, "text:\x00").Scan(&blank))
	require.Zero(t, blank)
	var key string
	require.NoError(t, s.DB.QueryRow(`SELECT play_key FROM source_plays`).Scan(&key))
	legacy, _ := json.Marshal(matchingRecord{ID: 2, Values: recordValues{Reason: "forwarding evidence", Music: &music.MatchIdentity{}}})
	_, err = s.DB.Exec(`INSERT INTO source_personal_evidence(play_key,record_id,details_json) VALUES(?,?,?)`, key, 2, string(legacy))
	require.NoError(t, err)
	_, err = s.DB.Exec(`INSERT INTO source_personal_evidence_music(music_key,play_key,record_id,position) VALUES(?,?,?,0)`, "text:\x00", key, 2)
	require.NoError(t, err)
	change := currentChange(t, s)
	tx, err := s.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repairPersonalEvidenceNames(ctx, tx, change.Config))
	require.NoError(t, tx.Commit())
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_personal_evidence_music WHERE music_key=?`, "text:\x00").Scan(&blank))
	require.Zero(t, blank)
	require.Equal(t, 2, count(t, s, "source_play_records"))
	require.Equal(t, 2, count(t, s, "source_records"))
	keys := evidenceMusicKeys(recordValues{Artist: "Artist", Title: "Song", Music: &music.MatchIdentity{}}, Rule{})
	require.Contains(t, keys, "text:\x00", "Explicit comparison names on identified music retain their matching semantics")
}

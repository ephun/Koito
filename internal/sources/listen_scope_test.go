package sources

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListenScopeFollowsContributingRecordThroughRebuildAndReimport(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	input := []byte("[" + complete + "]")
	mustImport(t, s, input, "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	var id int64
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT id,payload FROM source_records ORDER BY id LIMIT 1`).Scan(&id, &payload))
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "selected-record", Type: "inclusion", Subtype: "scope", Enabled: true, Program: namingFixture(t, fmt.Sprintf(`{"steps":[{"if":{"op":"in","args":[{"op":"literal","value":%d},{"op":"field","field":"play.record_ids"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`, id))})
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.NotEmpty(t, preview.ScopeExamples)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		var visible, stored int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM displayed_listens`).Scan(&visible))
		require.Zero(t, visible)
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&stored))
		require.Equal(t, 1, stored)
		var retained string
		require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE id=?`, id).Scan(&retained))
		require.Equal(t, payload, retained)
		mustImport(t, s, input, "renamed.json", "")
		_, err = s.Rebuild(ctx)
		require.NoError(t, err)
	}
	change = currentChange(t, s)
	change.Config.Rules[len(change.Config.Rules)-1].Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	var visible int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM displayed_listens`).Scan(&visible))
	require.Equal(t, 1, visible)
}

func TestListenScopeFollowsRecordThroughMatchAndSplit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "record", Type: "inclusion", Subtype: "scope", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"in","args":[{"op":"literal","value":1},{"op":"field","field":"play.record_ids"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`)})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	check := func(want int) {
		var visible int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM displayed_listens`).Scan(&visible))
		require.Equal(t, want, visible)
		require.Equal(t, 2, count(t, s, "source_records"))
	}
	check(0)
	for _, test := range []struct {
		action          string
		visible, stored int
	}{{"split", 1, 2}, {"match", 0, 1}} {
		change = currentChange(t, s)
		change.Decision = &DecisionChange{Left: 1, Right: 2, Action: test.action}
		versions := count(t, s, "filter_versions")
		decisions := count(t, s, "match_decisions")
		stored := count(t, s, "listens")
		preview, e := s.Preview(ctx, change)
		require.NoError(t, e)
		require.NotEmpty(t, preview.ScopeExamples)
		require.Equal(t, versions, count(t, s, "filter_versions"))
		require.Equal(t, decisions, count(t, s, "match_decisions"))
		require.Equal(t, stored, count(t, s, "listens"))
		change.Token = preview.Token
		_, err = s.Apply(ctx, change)
		require.NoError(t, err)
		check(test.visible)
		require.Equal(t, test.stored, count(t, s, "listens"))
		_, err = s.Rebuild(ctx)
		require.NoError(t, err)
		check(test.visible)
	}
}

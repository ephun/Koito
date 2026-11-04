package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gabehf/koito/internal/filters"
)

// Album grouping changes reporting membership. Catalog releases and every
// listen's release remain independent of these derived links.
func GroupingSchema() filters.Schema {
	s := MatchingSchema()
	s.Targets = map[string]filters.Type{"group.album": filters.Text, "group.various_artists": filters.Boolean}
	s.Fields["original.various_artists"] = filters.Boolean
	s.Labels = map[string]string{"catalog.kind": "Item type", "catalog.id": "Item identifier", "catalog.artist_ids": "Artist identifiers", "original.artists": "Original artist credits", "group.various_artists": "Various Artists", "original.various_artists": "Original Various Artists classification", "group.album": "Album group", "catalog.album_id": "Album identifier"}
	s.Help = map[string]string{"group.album": "Choose a reporting name for editions by the same artists. Empty text keeps this contribution separate."}
	s.Fields["catalog.album_id"] = filters.Number
	s.Fields["catalog.artist_ids"] = filters.List
	s.Fields["original.artists"] = filters.List
	for _, key := range []string{"display.artist", "display.album", "display.title"} {
		s.Fields[key] = filters.Text
	}
	return s
}

type groupingProgram struct {
	filter   IdentityFilter
	compiled *filters.CompiledProgram
}
type GroupingPrograms struct{ programs []groupingProgram }

func (p GroupingPrograms) Active() bool { return len(p.programs) > 0 }

func CompileGroupingPrograms(rules []IdentityFilter) (GroupingPrograms, error) {
	p := GroupingPrograms{}
	for _, r := range rules {
		if !r.Enabled || r.Type != "presentation" || r.Subtype != "grouping" {
			continue
		}
		if r.Program == nil {
			return p, fmt.Errorf("Grouping filter %s needs IF and SET blocks.", r.ID)
		}
		compiled, err := filters.CompileProgram(*r.Program, GroupingSchema())
		if err != nil {
			return p, err
		}
		p.programs = append(p.programs, groupingProgram{r, compiled})
	}
	// Presentation filters run in their explicit list order, like Naming.
	return p, nil
}

type GroupingResult struct {
	Album           string
	AlbumAssigned   bool
	VariousArtists  bool
	VariousAssigned bool
}

func (p GroupingPrograms) Resolve(ctx context.Context, env filters.Env) (string, bool, []IdentityIssue, error) {
	result, issues, err := p.ResolveValues(ctx, env)
	return result.Album, result.AlbumAssigned, issues, err
}
func (p GroupingPrograms) ResolveValues(ctx context.Context, env filters.Env) (GroupingResult, []IdentityIssue, error) {
	local := filters.Env{}
	for key, value := range env {
		local[key] = value
	}
	local["group.album"] = filters.String("")
	if _, ok := local["group.various_artists"]; !ok {
		local["group.various_artists"] = local["original.various_artists"]
	}
	result := GroupingResult{}
	result.VariousArtists, _ = local["group.various_artists"].Bool()
	issues := []IdentityIssue{}
	for _, program := range p.programs {
		out, err := program.compiled.Run(ctx, local)
		if err != nil {
			return result, issues, err
		}
		for _, issue := range out.Issues {
			issues = append(issues, IdentityIssue{program.filter.ID, program.filter.Name, issue})
		}
		if len(out.Issues) > 0 {
			continue
		}
		if _, assigned := out.Values["group.various_artists"]; assigned {
			kind, _ := local["catalog.kind"].Text()
			if kind != "album" {
				issues = append(issues, IdentityIssue{program.filter.ID, program.filter.Name, filters.Issue{Path: "group.various_artists", Reason: "Various Artists needs an album context."}})
				continue
			}
		}
		if value, ok := out.Values["group.album"]; ok {
			result.Album, _ = value.Text()
			result.AlbumAssigned = true
			local["group.album"] = value
		}
		if value, ok := out.Values["group.various_artists"]; ok {
			result.VariousArtists, _ = value.Bool()
			result.VariousAssigned = true
			local["group.various_artists"] = value
		}
	}
	return result, issues, nil
}

func AlbumGroupingEnvironment(id int64, title string, various bool, metadata ...filters.Env) filters.Env {
	env := filters.Env{"catalog.kind": filters.String("album"), "catalog.id": filters.Integer(id), "catalog.album_id": filters.Integer(id), "source.service": filters.String("koito"), "original.album": filters.String(title), "display.album": filters.String(title), "original.various_artists": filters.Bool(various), "group.various_artists": filters.Bool(various)}
	for _, extra := range metadata {
		for key, value := range extra {
			env[key] = value
		}
	}
	if artists, ok := env["original.artists"].Items(); ok && len(artists) == 1 {
		env["original.artist"] = artists[0]
		env["display.artist"] = artists[0]
	} else {
		delete(env, "original.artist")
		delete(env, "display.artist")
	}
	return env
}
func NativeAlbumGroupingEnvironment(id int64, title, artist, credits string, various bool) (filters.Env, error) {
	ids, err := filters.FromJSON(json.RawMessage("[" + credits + "]"))
	if err != nil {
		return nil, err
	}
	values, _ := ids.Items()
	names := filters.Values()
	if len(values) == 1 {
		names = filters.Values(filters.String(artist))
	} else {
		names, err = filters.FromJSON(json.RawMessage(artist))
		if err != nil {
			return nil, err
		}
	}
	return AlbumGroupingEnvironment(id, title, various, filters.Env{"catalog.artist_ids": ids, "original.artists": names}), nil
}
func groupingSets(steps []filters.Statement, target string) bool {
	for _, step := range steps {
		if step.Set == target || groupingSets(step.Then, target) || groupingSets(step.Else, target) {
			return true
		}
		for _, branch := range step.ElseIf {
			if groupingSets(branch.Then, target) {
				return true
			}
		}
	}
	return false
}
func (p GroupingPrograms) WritesClassification(id string) bool {
	for _, program := range p.programs {
		if program.filter.ID == id {
			return groupingSets(program.filter.Program.Steps, "group.various_artists")
		}
	}
	return false
}

type AlbumGroupingInput struct {
	PlayKey      string
	AggregateKey string
	ReleaseID    int64
	Env          func() filters.Env
}
type groupingRelease struct {
	id                     int64
	various                bool
	title, artist, credits string
}
type groupingRequest struct {
	release         groupingRelease
	label           string
	explicit        bool
	play, aggregate string
}

func albumGroupKey(credits, label string) string {
	raw, _ := json.Marshal([]string{credits, label})
	return string(raw)
}

// RefreshAlbumGroups publishes native defaults and per-contribution overrides
// together. Source/file exceptions can split contributions from one release.
func RefreshAlbumGroups(ctx context.Context, db Database, config string, inputs []AlbumGroupingInput) error {
	var settings struct {
		Rules []IdentityFilter `json:"rules"`
	}
	if err := json.Unmarshal([]byte(config), &settings); err != nil {
		return err
	}
	programs, err := CompileGroupingPrograms(settings.Rules)
	if err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM source_play_groupings`, `DELETE FROM source_aggregate_groupings`, `DELETE FROM native_release_groups`, `DELETE FROM album_group_members`} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if len(programs.programs) == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `SELECT r.id,r.title,
 COALESCE((SELECT group_concat(artist_id,',') FROM (SELECT artist_id FROM artist_releases WHERE release_id=r.id ORDER BY artist_id)),''),
 COALESCE((SELECT json_group_array(name) FROM (SELECT a.name FROM artist_releases ar JOIN artists_with_name a ON a.id=ar.artist_id WHERE ar.release_id=r.id ORDER BY ar.artist_id)),'[]')
 ,r.various_artists FROM releases_with_title r ORDER BY r.id`)
	if err != nil {
		return err
	}
	releases := map[int64]groupingRelease{}
	order := []int64{}
	for rows.Next() {
		var r groupingRelease
		var raw string
		if err = rows.Scan(&r.id, &r.title, &r.credits, &raw, &r.various); err != nil {
			rows.Close()
			return err
		}
		var names []string
		if err = json.Unmarshal([]byte(raw), &names); err != nil {
			rows.Close()
			return err
		}
		if len(names) == 1 {
			r.artist = names[0]
		} else {
			r.artist = raw
		}
		releases[r.id] = r
		order = append(order, r.id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	requests := make([]groupingRequest, 0, len(order)+len(inputs))
	nativeLabels := map[int64]string{}
	for _, id := range order {
		r := releases[id]
		env, e := NativeAlbumGroupingEnvironment(id, r.title, r.artist, r.credits, r.various)
		if e != nil {
			return e
		}
		label, explicit, _, e := programs.Resolve(ctx, env)
		if e != nil {
			return e
		}
		requests = append(requests, groupingRequest{release: r, label: label, explicit: explicit})
		if explicit {
			nativeLabels[id] = label
		}
	}
	for _, input := range inputs {
		r, ok := releases[input.ReleaseID]
		if !ok {
			return fmt.Errorf("Grouping release %d is unavailable.", input.ReleaseID)
		}
		env := input.Env()
		env["catalog.album_id"] = filters.Integer(r.id)
		label, explicit, _, e := programs.Resolve(ctx, env)
		if e != nil {
			return e
		}
		if !explicit {
			if value, ok := nativeLabels[r.id]; ok {
				label = value
				explicit = true
			}
		}
		requests = append(requests, groupingRequest{r, label, explicit, input.PlayKey, input.AggregateKey})
	}
	// Only explicit nonempty assignments create groups. An unchanged base title
	// can then join that group. Explicit empty assignments remain exceptions.
	wanted := map[string]*groupingRequest{}
	for i := range requests {
		r := &requests[i]
		if r.explicit && r.label != "" {
			key := albumGroupKey(r.release.credits, r.label)
			prior := wanted[key]
			if prior == nil || r.release.title == r.label && prior.release.title != prior.label || r.release.title == prior.release.title && r.release.id < prior.release.id {
				wanted[key] = r
			}
		}
	}
	for i := range requests {
		r := &requests[i]
		if r.explicit && r.label != r.release.title {
			continue
		}
		key := albumGroupKey(r.release.credits, r.release.title)
		if prior := wanted[key]; prior != nil && (prior.release.title != prior.label || r.release.id < prior.release.id) {
			copy := *r
			copy.label = r.release.title
			wanted[key] = &copy
		}
	}
	keys := make([]string, 0, len(wanted))
	for key := range wanted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ids := map[string]int64{}
	for _, key := range keys {
		r := wanted[key]
		var id int64
		if _, err = db.ExecContext(ctx, `INSERT INTO album_groups(group_key,title,canonical_release_id) VALUES(?,?,?) ON CONFLICT(group_key) DO UPDATE SET title=excluded.title,canonical_release_id=excluded.canonical_release_id`, key, r.label, r.release.id); err != nil {
			return err
		}
		if err = db.QueryRowContext(ctx, `SELECT id FROM album_groups WHERE group_key=?`, key).Scan(&id); err != nil {
			return err
		}
		ids[key] = id
	}
	queries := map[string]string{
		"member":    `INSERT INTO album_group_members(group_id,release_id) VALUES(?,?) ON CONFLICT DO NOTHING`,
		"play":      `INSERT INTO source_play_groupings(play_key,group_id) VALUES(?,?)`,
		"aggregate": `INSERT INTO source_aggregate_groupings(aggregate_key,group_id) VALUES(?,?)`,
		"native":    `INSERT INTO native_release_groups(release_id,group_id) VALUES(?,?)`,
	}
	statements := map[string]*sql.Stmt{}
	if preparer, ok := db.(interface {
		PrepareContext(context.Context, string) (*sql.Stmt, error)
	}); ok {
		for kind, query := range queries {
			statement, e := preparer.PrepareContext(ctx, query)
			if e != nil {
				return e
			}
			statements[kind] = statement
			defer statement.Close()
		}
	}
	execute := func(kind string, args ...any) error {
		var e error
		if statement := statements[kind]; statement != nil {
			_, e = statement.ExecContext(ctx, args...)
		} else {
			_, e = db.ExecContext(ctx, queries[kind], args...)
		}
		return e
	}
	members := map[[2]int64]bool{}
	for _, r := range requests {
		label := r.label
		if !r.explicit {
			label = r.release.title
		}
		id := ids[albumGroupKey(r.release.credits, label)]
		if r.explicit && r.label == "" {
			id = 0
		}
		if pair := [2]int64{id, r.release.id}; id > 0 && !members[pair] {
			if err = execute("member", id, r.release.id); err != nil {
				return err
			}
			members[pair] = true
		}
		switch {
		case r.play != "":
			err = execute("play", r.play, id)
		case r.aggregate != "":
			err = execute("aggregate", r.aggregate, id)
		case id > 0:
			err = execute("native", r.release.id, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// AssignAlbumGroup applies current native defaults to a newly captured release.
// Existing source overrides remain untouched.
func AssignAlbumGroup(ctx context.Context, db Database, releaseID int64) error {
	var config string
	err := db.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&config)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var settings struct {
		Rules []IdentityFilter `json:"rules"`
	}
	if err = json.Unmarshal([]byte(config), &settings); err != nil {
		return err
	}
	programs, err := CompileGroupingPrograms(settings.Rules)
	if err != nil || len(programs.programs) == 0 {
		return err
	}
	var title, credits, raw string
	var various bool
	err = db.QueryRowContext(ctx, `SELECT r.title,COALESCE((SELECT group_concat(artist_id,',') FROM (SELECT artist_id FROM artist_releases WHERE release_id=r.id ORDER BY artist_id)),''),COALESCE((SELECT json_group_array(name) FROM (SELECT a.name FROM artist_releases ar JOIN artists_with_name a ON a.id=ar.artist_id WHERE ar.release_id=r.id ORDER BY ar.artist_id)),'[]'),r.various_artists FROM releases_with_title r WHERE r.id=?`, releaseID).Scan(&title, &credits, &raw, &various)
	if err != nil {
		return err
	}
	var names []string
	if err = json.Unmarshal([]byte(raw), &names); err != nil {
		return err
	}
	artist := raw
	if len(names) == 1 {
		artist = names[0]
	}
	env, err := NativeAlbumGroupingEnvironment(releaseID, title, artist, credits, various)
	if err != nil {
		return err
	}
	result, issues, err := programs.ResolveValues(ctx, env)
	label, explicit := result.Album, result.AlbumAssigned
	if err == nil && result.VariousAssigned {
		_, err = db.ExecContext(ctx, `INSERT INTO album_classification(album_id,various_artists) VALUES(?,?) ON CONFLICT(album_id) DO UPDATE SET various_artists=excluded.various_artists`, releaseID, result.VariousArtists)
	}
	if err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM album_classification_issues WHERE album_id=?`, releaseID); err != nil {
		return err
	}
	relevant := []IdentityIssue{}
	for _, issue := range issues {
		if programs.WritesClassification(issue.ID) {
			relevant = append(relevant, issue)
		}
	}
	if len(relevant) > 0 {
		raw, e := json.Marshal(relevant)
		if e != nil {
			return e
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO album_classification_issues(album_id,issues_json) VALUES(?,?)`, releaseID, string(raw)); err != nil {
			return err
		}
	}
	if !explicit {
		label = title
	}
	if label == "" {
		return nil
	}
	key := albumGroupKey(credits, label)
	if explicit {
		if _, err = db.ExecContext(ctx, `INSERT INTO album_groups(group_key,title,canonical_release_id) VALUES(?,?,?) ON CONFLICT(group_key) DO NOTHING`, key, label, releaseID); err != nil {
			return err
		}
	}
	var id int64
	err = db.QueryRowContext(ctx, `SELECT id FROM album_groups WHERE group_key=? AND (EXISTS(SELECT 1 FROM album_group_members WHERE group_id=album_groups.id) OR ?=1)`, key, explicit).Scan(&id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO native_release_groups(release_id,group_id) VALUES(?,?) ON CONFLICT(release_id) DO UPDATE SET group_id=excluded.group_id`, releaseID, id); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO album_group_members(group_id,release_id) VALUES(?,?) ON CONFLICT DO NOTHING`, id, releaseID); err != nil {
		return err
	}
	// When a deluxe edition creates a group, an already captured base release
	// can join immediately. Evaluate it too so explicit exceptions still win.
	rows, err := db.QueryContext(ctx, `SELECT r.id,r.various_artists FROM releases_with_title r WHERE r.title=? AND COALESCE((SELECT group_concat(artist_id,',') FROM (SELECT artist_id FROM artist_releases WHERE release_id=r.id ORDER BY artist_id)),'')=? ORDER BY r.id`, label, credits)
	if err != nil {
		return err
	}
	candidates := []int64{}
	candidateFlags := map[int64]bool{}
	for rows.Next() {
		var candidate int64
		var flag bool
		if err = rows.Scan(&candidate, &flag); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, candidate)
		candidateFlags[candidate] = flag
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var canonical int64
	for _, candidate := range candidates {
		env["catalog.id"] = filters.Integer(candidate)
		env["original.various_artists"] = filters.Bool(candidateFlags[candidate])
		env["group.various_artists"] = filters.Bool(candidateFlags[candidate])
		env["catalog.album_id"], env["original.album"], env["display.album"] = filters.Integer(candidate), filters.String(label), filters.String(label)
		target, assigned, _, e := programs.Resolve(ctx, env)
		if e != nil {
			return e
		}
		if assigned && target != label {
			continue
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO native_release_groups(release_id,group_id) VALUES(?,?) ON CONFLICT(release_id) DO UPDATE SET group_id=excluded.group_id`, candidate, id); err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO album_group_members(group_id,release_id) VALUES(?,?) ON CONFLICT DO NOTHING`, id, candidate); err != nil {
			return err
		}
		if canonical == 0 || candidate < canonical {
			canonical = candidate
		}
	}
	if canonical > 0 {
		_, err = db.ExecContext(ctx, `UPDATE album_groups SET canonical_release_id=? WHERE id=?`, canonical, id)
	}
	return err
}

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

type matchingRecord struct {
	ContextIssues    []FilterIssue      `json:"-"`
	GameCatalog      *music.GameCatalog `json:"-"`
	LibraryFields    filters.Env        `json:"-"`
	ExternalID       string             `json:"external_id,omitempty"`
	CompletionIssues []FilterIssue      `json:"-"`
	ValidityIssues   []FilterIssue      `json:"-"`
	RepetitionIssues []FilterIssue      `json:"-"`
	MusicIssues      []FilterIssue      `json:"-"`
	TimeIssues       []FilterIssue      `json:"-"`
	timeOffset       *time.Duration
	ID               int64        `json:"id"`
	SourceID         int64        `json:"source_id"`
	Service          string       `json:"service"`
	FileHash         string       `json:"file_hash"`
	Ordinal          int          `json:"ordinal"`
	Parser           string       `json:"parser"`
	Kind             string       `json:"kind,omitempty"`
	Payload          string       `json:"payload,omitempty"`
	UserID           int32        `json:"-"`
	Values           recordValues `json:"values"`
	Start            time.Time    `json:"time"`
	Basis            string       `json:"time_basis"`
	Device           string       `json:"device,omitempty"`
	LibraryID        string       `json:"library_id,omitempty"`
	TrackID          string       `json:"track_id,omitempty"`
	RawFields        filters.Env  `json:"-"`
}

type MatchDecision struct {
	ID        int64  `json:"id"`
	Left      int64  `json:"left"`
	Right     int64  `json:"right"`
	Action    string `json:"action"`
	CreatedAt int64  `json:"created_at"`
}

type plannedPlay struct {
	Key            string      `json:"key"`
	Records        []int64     `json:"records"`
	Representative int64       `json:"representative"`
	Reason         string      `json:"reason"`
	At             time.Time   `json:"at,omitempty"`
	Evidence       string      `json:"evidence,omitempty"`
	IntervalStart  time.Time   `json:"interval_start,omitempty"`
	IntervalEnd    time.Time   `json:"interval_end,omitempty"`
	Attribution    Attribution `json:"attribution"`
}

type matchPlan struct {
	Records      []matchingRecord
	Plays        []plannedPlay
	Reasons      map[int64]string
	Keys         map[int64]string
	Ambiguous    map[int64][]int64
	Aggregates   []AggregateContribution
	Attributions map[string]Attribution
}

func loadMatchingRecords(ctx context.Context, reader sqlReader, config RuleConfig, extra ...RuleConfig) ([]matchingRecord, error) {
	if err := prepareValidityPrograms(config); err != nil {
		return nil, err
	}
	if err := prepareContextPrograms(config); err != nil {
		return nil, err
	}
	if err := prepareCompletionPrograms(config); err != nil {
		return nil, err
	}
	if err := prepareDirectPrograms(config); err != nil {
		return nil, err
	}
	if err := prepareAggregatePrograms(config); err != nil {
		return nil, err
	}
	if err := prepareHeuristicPrograms(config); err != nil {
		return nil, err
	}
	if err := prepareCountPrograms(config); err != nil {
		return nil, err
	}
	if err := prepareTimePrograms(config); err != nil {
		return nil, err
	}
	if err := preparePlayPrograms(config); err != nil {
		return nil, err
	}
	musicPrograms, err := compileMusicPrograms(config)
	if err != nil {
		return nil, err
	}
	rows, err := reader.QueryContext(ctx, `SELECT r.id,r.source_id,a.service,r.file_hash,r.ordinal,r.parser,r.payload,a.user_id,r.kind FROM source_records r JOIN source_accounts a ON a.id=r.source_id ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fieldConfig := RuleConfig{Rules: append([]Rule(nil), config.Rules...)}
	for _, additional := range extra {
		fieldConfig.Rules = append(fieldConfig.Rules, additional.Rules...)
	}
	rawFields := requestedRawFields(fieldConfig)
	countPrograms := hasCountPrograms(fieldConfig)
	out := []matchingRecord{}
	for rows.Next() {
		var r matchingRecord
		if err = rows.Scan(&r.ID, &r.SourceID, &r.Service, &r.FileHash, &r.Ordinal, &r.Parser, &r.Payload, &r.UserID, &r.Kind); err != nil {
			return nil, err
		}
		if err = retainRawFields(&r, rawFields); err != nil {
			return nil, fmt.Errorf("Read source fields for record %d: %w", r.ID, err)
		}
		r.Values = normalizeRecord([]byte(r.Payload), r.Parser, config.zoneFor(r.SourceID, r.FileHash, r.Service))
		if snapshotParser(r.Parser) {
			r.ExternalID = snapshotExternalID(r)
		}
		if countPrograms && (r.Service == "itunes" || r.Service == "navidrome") {
			if err = retainCountRaw(&r); err != nil {
				return nil, err
			}
		}
		// Matching needs interpreted values; inspection reads the original payload
		// separately. Avoid retaining a second full copy of large history exports.
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err = attachRecordMemberships(ctx, reader, out); err != nil {
		return nil, err
	}
	attachGameCatalog(out)
	for i := range out {
		r := &out[i]
		interpretNormalizedRecord(r, config)
		if err = applyMusicPrograms(ctx, r, musicPrograms); err != nil {
			return nil, err
		}
		r.Payload = ""
	}
	selectConcertRevisions(out)
	return out, nil
}

func interpretMatchingRecord(r *matchingRecord, config RuleConfig) {
	r.Values = normalizeRecord(json.RawMessage(r.Payload), r.Parser, config.zoneFor(r.SourceID, r.FileHash, r.Service))
	interpretNormalizedRecord(r, config)
}

// Bulk loading has already decoded the original fields for membership lookup.
func interpretNormalizedRecord(r *matchingRecord, config RuleConfig) {
	r.TimeIssues = nil
	r.timeOffset = nil
	r.Start = r.Values.Time
	r.Basis = r.Values.TimeMeaning
	r.Device, r.LibraryID, r.TrackID = r.Values.Device, r.Values.NativeLibraryID, r.Values.NativeTrackID
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(r.Payload), &fields)
	for _, key := range []string{"device_name", "device", "platform"} {
		if value := rawString(fields[key]); value != "" {
			r.Device = value
			break
		}
	}
	if r.Service == "itunes" {
		var row libraryEnvelope
		_ = json.Unmarshal([]byte(r.Payload), &row)
		r.LibraryID = row.LibraryID
		r.TrackID = libraryText(row.Fields, "Persistent ID")
		r.Device = libraryText(row.Fields, "Device")
	}
	applyContextPrograms(r, config)
	basis := applyObservedTime(r, config)
	applyValidity(r, config)
	applyCompletion(r, config)
	if r.Kind == "now_playing" {
		r.Values.NowPlaying = true
		r.Values.Reason = "now playing"
	}
	if (r.Service == "spotify" || r.Values.OriginalFormat == Parser || r.Values.OriginalFormat == SpotifyStandard) && r.Values.TimeMeaning == "end" && basis == "estimated_start" && r.Values.Reason == "counted" {
		// Subtract actual played time, never an assumed recording duration. Guard
		// duration conversion against overflow and implausible pre-epoch results.
		if r.Values.PlayedMS > r.Start.UnixMilli() || r.Values.PlayedMS > int64((1<<63-1)/int64(time.Millisecond)) {
			r.Values.Reason = "invalid played duration"
		} else {
			r.Start = r.Start.Add(-time.Duration(r.Values.PlayedMS) * time.Millisecond)
			r.Basis = "estimated start"
		}
	}
	if r.timeOffset != nil && !r.Start.IsZero() {
		r.Start = r.Start.Add(*r.timeOffset)
		r.Basis = "adjusted time"
	}
	if r.Values.Performance != nil {
		r.Values.Performance.Catalog = r.GameCatalog
	}
}

func loadDecisions(ctx context.Context, reader sqlReader) ([]MatchDecision, error) {
	rows, err := reader.QueryContext(ctx, `SELECT id,left_record,right_record,action,created_at FROM match_decisions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MatchDecision{}
	for rows.Next() {
		var d MatchDecision
		if err = rows.Scan(&d.ID, &d.Left, &d.Right, &d.Action, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func pairKey(a, b int64) [2]int64 {
	if a > b {
		a, b = b, a
	}
	return [2]int64{a, b}
}

// Use the same Unicode equivalence as EqualFold when building candidate keys.
// Lowercasing alone misses pairs such as Greek final and ordinary sigma.
func foldedName(text string) string {
	return strings.Map(func(r rune) rune {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		return minimum
	}, text)
}

func musicCompatible(a, b recordValues, rule Rule) bool {
	// Conflicting reliable IDs outweigh similar text. A common reliable ID can
	// corroborate a recording while retaining differing source album fields.
	if a.URI != "" && b.URI != "" {
		if a.URI != b.URI {
			return false
		}
	}
	if a.RecordingID != "" && b.RecordingID != "" {
		if a.RecordingID != b.RecordingID {
			return false
		}
	}
	if (a.URI != "" && b.URI != "") || (a.RecordingID != "" && b.RecordingID != "") {
		return true
	}
	if a.Music != nil || b.Music != nil {
		left, right := comparisonMusic(a, rule), comparisonMusic(b, rule)
		return left.Artist == right.Artist && left.Title == right.Title && (left.Album == right.Album || left.Album == "" && left.MissingAlbum || right.Album == "" && right.MissingAlbum)
	}
	equal := func(a, b string) bool {
		if rule.Enabled && !rule.Settings.CaseSensitive {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	if !equal(a.Artist, b.Artist) || !equal(a.Title, b.Title) {
		return false
	}
	if equal(a.Album, b.Album) {
		return true
	}
	return rule.Enabled && rule.Settings.MatchMissingAlbum && (a.Album == "" || b.Album == "")
}

func comparisonMusic(v recordValues, rule Rule) music.MatchIdentity {
	if v.Music != nil {
		return *v.Music
	}
	identity := music.MatchIdentity{Artist: v.Artist, Album: v.Album, Title: v.Title, MissingAlbum: rule.Program == nil && rule.Enabled && rule.Settings.MatchMissingAlbum}
	if rule.Program == nil && rule.Enabled && !rule.Settings.CaseSensitive {
		identity.Artist = foldedName(identity.Artist)
		identity.Album = foldedName(identity.Album)
		identity.Title = foldedName(identity.Title)
	}
	return identity
}

func timeCompatible(a, b matchingRecord, tolerance time.Duration) bool {
	if !comparableTimeBasis(a.Basis, b.Basis) {
		return false
	}
	ae := a.Start.Add(time.Duration(a.Values.Precision))
	be := b.Start.Add(time.Duration(b.Values.Precision))
	if a.Start.Before(be) && b.Start.Before(ae) {
		return true
	}
	return tolerance > 0 && !a.Start.After(be.Add(tolerance)) && !b.Start.After(ae.Add(tolerance))
}

func comparableTimeBasis(a, b string) bool {
	// Preserve established estimated-start and adjusted-time behavior. A
	// submission or unknown timestamp needs an explicit interpretation first.
	if a == "unknown" || b == "unknown" {
		return false
	}
	if a == "submission" || b == "submission" {
		return a == b
	}
	if a == "result" || b == "result" || a == "date" || b == "date" {
		return a == b
	}
	return (a == "end") == (b == "end")
}

// calculateMatches does not write catalog, rules, decisions or display rows.
// Exact source duplicates are linked first. Other automatic matches require
// a unique partner on both sides; no repeated-pass or transitive time matching.
func calculateMatches(ctx context.Context, records []matchingRecord, config RuleConfig, decisions []MatchDecision) (matchPlan, error) {
	if err := preparePlayPrograms(config); err != nil {
		return matchPlan{}, err
	}
	p := matchPlan{Records: records, Reasons: map[int64]string{}, Keys: map[int64]string{}, Ambiguous: map[int64][]int64{}}
	musicRule, playRule := config.matchingRules()
	byID := map[int64]int{}
	parent := make([]int, len(records))
	members := map[int][]int{}
	for i, r := range records {
		byID[r.ID] = i
		parent[i] = i
		members[i] = []int{i}
		p.Reasons[r.ID] = r.Values.Reason
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	latest := map[[2]int64]MatchDecision{}
	for _, d := range decisions {
		latest[pairKey(d.Left, d.Right)] = d
	}
	splits := map[[2]int64]bool{}
	for k, d := range latest {
		if d.Action == "split" {
			splits[k] = true
		}
	}
	canJoin := func(a, b int) bool {
		for _, i := range members[a] {
			for _, j := range members[b] {
				if splits[pairKey(records[i].ID, records[j].ID)] {
					return false
				}
			}
		}
		return true
	}
	join := func(a, b int) bool {
		a, b = root(a), root(b)
		if a == b {
			return true
		}
		if !canJoin(a, b) {
			return false
		}
		if records[a].ID > records[b].ID {
			a, b = b, a
		}
		parent[b] = a
		members[a] = append(members[a], members[b]...)
		delete(members, b)
		return true
	}
	// Apply explicit decisions in stable order, independently of automatic rules.
	active := []MatchDecision{}
	for _, d := range latest {
		if d.Action == "match" {
			active = append(active, d)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	for _, d := range active {
		a, aok := byID[d.Left]
		b, bok := byID[d.Right]
		if !aok || !bok {
			return p, fmt.Errorf("Match refers to an unknown record.")
		}
		// Disabling time inference may temporarily exclude a manually linked record;
		// retain the decision for when that evidence becomes usable again.
		if records[a].Values.Reason != "counted" || records[b].Values.Reason != "counted" {
			continue
		}
		if !join(a, b) {
			return p, fmt.Errorf("Manual match conflicts with a saved split.")
		}
	}
	if playRule.Enabled {
		events := map[string][]int{}
		for i, r := range records {
			if r.Values.Reason == "counted" && r.Values.EventID != "" {
				key := fmt.Sprintf("%d:%s", r.SourceID, r.Values.EventID)
				for _, other := range events[key] {
					if resolvePlayPair(ctx, config, r, records[other]).Match {
						join(i, other)
					}
				}
				events[key] = append(events[key], i)
			}
		}
		exact := map[string][]int{}
		for i, r := range records {
			if r.Values.Reason != "counted" {
				continue
			}
			identity := []string{r.Values.Artist, r.Values.Title, r.Values.Album, r.Values.RecordingID}
			if r.Values.URI != "" {
				identity = []string{r.Values.URI, r.Values.RecordingID}
			}
			raw, _ := json.Marshal([]any{r.SourceID, r.Values.Time.UTC().Format(time.RFC3339Nano), r.Values.Precision, identity})
			key := string(raw)
			exact[key] = append(exact[key], i)
		}
		exactKeys := make([]string, 0, len(exact))
		for key := range exact {
			exactKeys = append(exactKeys, key)
		}
		sort.Strings(exactKeys)
		for _, key := range exactKeys {
			indices := exact[key]
			first := records[indices[0]]
			if first.Values.Precision > int64(time.Second) {
				counts := map[string]int{}
				uncertain := false
				for _, i := range indices {
					counts[records[i].FileHash]++
					if counts[records[i].FileHash] > 1 {
						uncertain = true
					}
				}
				// A minute can contain genuine repeated short tracks. Multiplicity
				// in any export makes this bucket ambiguous across all its copies.
				if uncertain {
					continue
				}
			}
			representatives := []int{}
			for _, i := range indices {
				linked := false
				for _, other := range representatives {
					if resolvePlayPair(ctx, config, records[i], records[other]).Match && join(i, other) {
						linked = true
						break
					}
				}
				if !linked {
					representatives = append(representatives, i)
				}
			}
		}
		// Candidate blocks cover exact names and explicit IDs. Sort by time so
		// distant repeats never require a quadratic all-history comparison.
		blocks := map[string][]int{}
		for i, r := range records {
			if r.Values.Reason != "counted" {
				continue
			}
			identity := comparisonMusic(r.Values, musicRule)
			name := []string{identity.Artist, identity.Title}
			raw, _ := json.Marshal(name)
			keys := []string{"name:" + string(raw)}
			if r.Values.URI != "" {
				keys = append(keys, "uri:"+r.Values.URI)
			}
			if r.Values.RecordingID != "" {
				keys = append(keys, "recording:"+r.Values.RecordingID)
			}
			for _, key := range keys {
				blocks[key] = append(blocks[key], i)
			}
		}
		candidates := map[int]map[int]bool{}
		blockedPairs := map[[2]int]bool{}
		candidatePair := func(a, b int) [2]int {
			if a > b {
				a, b = b, a
			}
			return [2]int{a, b}
		}
		tolerance := playSearchGap(config)
		for _, block := range blocks {
			sort.Slice(block, func(i, j int) bool { return records[block[i]].Start.Before(records[block[j]].Start) })
			for pos, i := range block {
				if err := ctx.Err(); err != nil {
					return p, err
				}
				a := records[i]
				for _, j := range block[pos+1:] {
					b := records[j]
					if tolerance != unlimitedPairGap && b.Start.After(a.Start.Add(time.Duration(a.Values.Precision)).Add(tolerance)) {
						break
					}
					ra, rb := root(i), root(j)
					if ra == rb || a.UserID != b.UserID || !canJoin(ra, rb) {
						continue
					}
					if a.Service == b.Service && (a.SourceID != b.SourceID || (a.FileHash == b.FileHash && (a.Service != "lastfm" || a.Values.Precision <= int64(time.Second) || b.Values.Precision <= int64(time.Second)))) {
						continue
					}
					if !musicCompatible(a.Values, b.Values, musicRule) || !resolvePlayPair(ctx, config, a, b).Match {
						continue
					}
					// Every member must remain compatible with the candidate. This avoids
					// merging conflicting known albums through a third, sparse record.
					compatible := true
					sharedFile := false
					for _, x := range members[ra] {
						for _, y := range members[rb] {
							if records[x].FileHash == records[y].FileHash && records[x].Service == records[y].Service {
								sharedFile = true
							}
							if !musicCompatible(records[x].Values, records[y].Values, musicRule) || !resolvePlayPair(ctx, config, records[x], records[y]).Match {
								compatible = false
							}
						}
					}
					if !compatible {
						continue
					}
					if candidates[ra] == nil {
						candidates[ra] = map[int]bool{}
					}
					if candidates[rb] == nil {
						candidates[rb] = map[int]bool{}
					}
					candidates[ra][rb] = true
					candidates[rb][ra] = true
					if sharedFile {
						blockedPairs[candidatePair(ra, rb)] = true
					}
				}
			}
		}
		// Evaluate complete components before joining. Three services can all
		// describe one event, but a chain or competing event remains ambiguous.
		visited := map[int]bool{}
		for first := range candidates {
			if visited[first] {
				continue
			}
			component := []int{first}
			visited[first] = true
			for pos := 0; pos < len(component); pos++ {
				for next := range candidates[component[pos]] {
					if !visited[next] {
						visited[next] = true
						component = append(component, next)
					}
				}
			}
			if len(component) < 3 {
				continue
			}
			complete := true
			sources := map[int64]int{}
			for _, a := range component {
				if len(candidates[a]) != len(component)-1 {
					complete = false
				}
				for _, index := range members[a] {
					source := records[index].SourceID
					if previous, exists := sources[source]; exists && previous != a {
						complete = false
					}
					sources[source] = a
				}
				for _, b := range component {
					if a != b && (!canJoin(a, b) || blockedPairs[candidatePair(a, b)]) {
						complete = false
					}
				}
			}
			if complete {
				sort.Ints(component)
				for _, next := range component[1:] {
					join(component[0], next)
				}
				for _, node := range component {
					delete(candidates, node)
				}
			}
		}
		// Remaining components are evaluated before changing any groups, so a
		// chain cannot become unique because an earlier pair was removed.
		pairs := [][2]int{}
		for a, partners := range candidates {
			if len(partners) == 1 {
				for b := range partners {
					if len(candidates[b]) == 1 && a < b && !blockedPairs[candidatePair(a, b)] {
						pairs = append(pairs, [2]int{a, b})
					}
				}
			}
		}
		for _, pair := range pairs {
			join(pair[0], pair[1])
		}
		for a, partners := range candidates {
			for b := range partners {
				if root(a) == root(b) {
					continue
				}
				for _, i := range members[root(a)] {
					for _, j := range members[root(b)] {
						p.Ambiguous[records[i].ID] = append(p.Ambiguous[records[i].ID], records[j].ID)
					}
				}
			}
		}
	}
	for _, indices := range members {
		if records[indices[0]].Values.Reason != "counted" {
			continue
		}
		sort.Slice(indices, func(i, j int) bool { return records[indices[i]].ID < records[indices[j]].ID })
		play := plannedPlay{Key: digest([]byte(fmt.Sprintf("play:%d", records[indices[0]].ID))), Records: []int64{}, Reason: "counted"}
		representative := indices[0]
		for _, i := range indices {
			play.Records = append(play.Records, records[i].ID)
			if records[i].Service == playRule.Settings.PreferredSource && records[representative].Service != playRule.Settings.PreferredSource {
				representative = i
			}
			if records[i].Service == records[representative].Service && records[i].Values.Precision < records[representative].Values.Precision {
				representative = i
			}
		}
		play.Representative = records[representative].ID
		if len(indices) > 1 {
			play.Reason = "matched records"
		}
		for _, i := range indices {
			p.Keys[records[i].ID] = play.Key
			p.Reasons[records[i].ID] = play.Reason
			if len(p.Ambiguous[records[i].ID]) > 0 {
				p.Reasons[records[i].ID] = "ambiguous match; counted separately"
			}
		}
		p.Plays = append(p.Plays, play)
	}
	sort.Slice(p.Plays, func(i, j int) bool { return p.Plays[i].Records[0] < p.Plays[j].Records[0] })
	for id, partners := range p.Ambiguous {
		sort.Slice(partners, func(i, j int) bool { return partners[i] < partners[j] })
		unique := partners[:0]
		for _, partner := range partners {
			if len(unique) == 0 || unique[len(unique)-1] != partner {
				unique = append(unique, partner)
			}
		}
		p.Ambiguous[id] = unique
	}
	return p, nil
}

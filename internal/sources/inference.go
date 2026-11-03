package sources

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

type AggregateContribution struct {
	Key            string      `json:"key"`
	LibraryKey     string      `json:"library_key"`
	TrackKey       string      `json:"track_key"`
	Representative int64       `json:"representative"`
	Records        []int64     `json:"records"`
	Reported       *int64      `json:"reported"`
	Available      int64       `json:"available"`
	Attributed     int64       `json:"attributed"`
	Observed       int64       `json:"observed"`
	Overlap        int64       `json:"overlap"`
	CoveredPlays   []string    `json:"covered_plays"`
	LastPlayed     int64       `json:"last_played"`
	Estimated      int64       `json:"estimated"`
	Undated        int64       `json:"undated"`
	Attribution    Attribution `json:"attribution"`
	Rules          []string    `json:"rules"`
	Reason         string      `json:"reason"`
	From           time.Time   `json:"from"`
	Until          time.Time   `json:"until"`
}

// Count modes are explicit interpretations. In particular a decrease is not
// automatically proof of a reset, so reset_total must be deliberately selected.
func interpretedCount(snapshots []LibrarySnapshot, mode string) (int64, LibrarySnapshot, bool) {
	values := append([]LibrarySnapshot{}, snapshots...)
	sort.Slice(values, func(i, j int) bool {
		a, b := libraryDate(values[i].Snapshot), libraryDate(values[j].Snapshot)
		if !a.Equal(b) {
			return a.Before(b)
		}
		return values[i].RecordID < values[j].RecordID
	})
	var selected LibrarySnapshot
	if mode == "maximum" {
		for _, s := range values {
			if s.Count != nil && (selected.Count == nil || *s.Count > *selected.Count || *s.Count == *selected.Count && libraryDate(s.Snapshot).After(libraryDate(selected.Snapshot))) {
				selected = s
			}
		}
		if selected.Count == nil {
			return 0, selected, false
		}
		return *selected.Count, selected, true
	}
	if mode == "latest" {
		var latest time.Time
		conflict := false
		for _, s := range values {
			date := libraryDate(s.Snapshot)
			if date.IsZero() {
				continue
			}
			if date.After(latest) {
				selected = s
				latest = date
				conflict = false
			} else if date.Equal(latest) && (s.Count == nil || selected.Count == nil || *s.Count != *selected.Count) {
				conflict = true
			}
		}
		if selected.Count == nil || conflict {
			return 0, selected, false
		}
		return *selected.Count, selected, true
	}
	if mode != "reset_total" {
		return 0, selected, false
	}
	known := map[int64]bool{}
	var previous *int64
	var previousDate time.Time
	var completed int64
	for _, s := range values {
		date := libraryDate(s.Snapshot)
		if date.IsZero() {
			continue
		}
		if s.Count == nil {
			return 0, selected, false
		}
		value := *s.Count
		known[value] = true
		if previous != nil {
			if date.Equal(previousDate) && value != *previous {
				return 0, selected, false
			}
			if value < *previous {
				if *previous > math.MaxInt64-completed {
					return 0, selected, false
				}
				completed += *previous
			}
		}
		copy := value
		previous = &copy
		previousDate = date
		selected = s
	}
	if previous == nil || *previous > math.MaxInt64-completed {
		return 0, selected, false
	}
	for _, s := range values {
		if libraryDate(s.Snapshot).IsZero() && (s.Count == nil || !known[*s.Count]) {
			return 0, selected, false
		}
	}
	return completed + *previous, selected, true
}

func proportion(total int64, percent int) int64 {
	return total/100*int64(percent) + (total%100*int64(percent))/100
}

func aggregateAttribution(r matchingRecord, config RuleConfig, from, until time.Time, evidence evidenceIndex) Attribution {
	scopeUntil := until
	if !scopeUntil.IsZero() {
		scopeUntil = scopeUntil.Add(time.Nanosecond)
	}
	direct := directAttribution(r, config, from, scopeUntil)
	if direct.Basis != "unattributed library" {
		return direct
	}
	rule, conflict := chooseRule(config, "attribution", "aggregate", func(rule Rule) bool { return scopeContains(rule.Scope, r, from, scopeUntil) })
	if conflict {
		return Attribution{Listener: "unknown", Basis: "conflicting aggregate rules", Rules: []string{}}
	}
	if rule != nil {
		return Attribution{Listener: "partial", Basis: "aggregate", Rules: []string{rule.ID}, Share: rule.Settings.SharePercent}
	}
	rule, conflict = chooseRule(config, "attribution", "heuristic", func(rule Rule) bool { return scopeContains(rule.Scope, r, from, scopeUntil) })
	if conflict {
		return Attribution{Listener: "unknown", Basis: "conflicting heuristic rules", Rules: []string{}}
	}
	if rule != nil && evidenceMatches(evidence, r, rule.Settings.EvidenceSourceID, rule.Settings.MinimumMatches, config, from, until) {
		return Attribution{Listener: "partial", Basis: "heuristic", Rules: []string{rule.ID}, Share: rule.Settings.SharePercent}
	}
	return direct
}

// calculateView is shared by saving, previewing, and rebuilding. Matching keeps
// source records intact; attribution and inference only change contributions.
func calculateView(ctx context.Context, reader sqlReader, records []matchingRecord, config RuleConfig, decisions []MatchDecision) (matchPlan, error) {
	plan, err := calculateMatches(ctx, records, config, decisions)
	if err != nil {
		return plan, err
	}
	applyAttribution(&plan, config)
	active := false
	for _, rule := range config.Rules {
		if rule.Enabled && rule.Scope.Service == "itunes" && rule.Type == "inference" && rule.Subtype == "count" {
			active = true
		}
	}
	if !active {
		return plan, nil
	}
	libraries, err := libraryEvidence(ctx, reader)
	if err != nil {
		return plan, err
	}
	err = inferLibraryContributions(ctx, &plan, libraries, config)
	return plan, err
}

func inferLibraryContributions(ctx context.Context, plan *matchPlan, libraries []LibraryEvidence, config RuleConfig) error {
	byID := recordIndex(plan.Records)
	music, _ := config.matchingRules()
	evidence := buildPersonalEvidence(*plan, config)
	_, playMatching := config.matchingRules()
	claimedObserved := map[string]bool{}
	// All usable observed plays reserve their interval before estimates are
	// placed. Subsequent libraries also see already placed inferred events.
	reserved := map[string][]plannedPlay{}
	for _, p := range plan.Plays {
		r := byID[p.Representative]
		for _, key := range evidenceMusicKeys(r.Values, music) {
			reserved[key] = append(reserved[key], p)
		}
	}
	totalEstimates := int64(0)
	for _, library := range libraries {
		for _, track := range library.Tracks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(track.Snapshots) == 0 {
				continue
			}
			recordFor := func(s LibrarySnapshot) matchingRecord {
				r := byID[s.RecordID]
				r.LibraryID = library.LibraryID
				return r
			}
			countRule, conflict := chooseRule(config, "inference", "count", func(rule Rule) bool {
				for _, s := range track.Snapshots {
					if scopeContains(rule.Scope, recordFor(s), time.Time{}, time.Time{}) {
						return true
					}
				}
				return false
			})
			if countRule == nil {
				continue
			}
			contribution := AggregateContribution{Key: digest([]byte(library.Key + "\x00" + track.Key)), LibraryKey: library.Key, TrackKey: track.Key, Representative: track.Snapshots[0].RecordID, Reported: track.Reported, Records: []int64{}, Rules: []string{countRule.ID}, Attribution: Attribution{Listener: "unknown", Basis: "unattributed library", Rules: []string{}}, Reason: "count inference"}
			for _, s := range track.Snapshots {
				contribution.Records = append(contribution.Records, s.RecordID)
			}
			if conflict {
				contribution.Reason = "conflicting count rules"
				plan.Aggregates = append(plan.Aggregates, contribution)
				continue
			}
			if track.PersistentID == "" {
				contribution.Reason = "unknown persistent identity"
				plan.Aggregates = append(plan.Aggregates, contribution)
				continue
			}
			if library.LibraryID == "" && countRule.Scope.FileHash == "" {
				contribution.Reason = "choose a file for the unidentified library"
				plan.Aggregates = append(plan.Aggregates, contribution)
				continue
			}
			snapshots := []LibrarySnapshot{}
			for _, s := range track.Snapshots {
				if scopeContains(countRule.Scope, recordFor(s), time.Time{}, time.Time{}) {
					snapshots = append(snapshots, s)
				}
			}
			contribution.Records = []int64{}
			for _, s := range snapshots {
				contribution.Records = append(contribution.Records, s.RecordID)
			}
			sort.Slice(contribution.Records, func(i, j int) bool { return contribution.Records[i] < contribution.Records[j] })
			amount, selected, valid := interpretedCount(snapshots, countRule.Settings.CountMode)
			if !valid {
				contribution.Reason = "unresolved cumulative count"
				plan.Aggregates = append(plan.Aggregates, contribution)
				continue
			}
			contribution.Available = amount
			contribution.Representative = selected.RecordID
			r := recordFor(selected)
			from, lastPlayed := libraryDate(selected.Added), libraryDate(selected.LastPlayed)
			until := lastPlayed
			for _, issue := range selected.Issues {
				if issue == "invalid last played" || issue == "last played precedes date added" || issue == "track date exceeds snapshot" || issue == "last played with zero count" {
					lastPlayed = time.Time{}
				}
				if issue == "last played precedes date added" || issue == "track date exceeds snapshot" {
					from = time.Time{}
				}
			}
			until = lastPlayed
			if until.IsZero() {
				until = libraryDate(selected.Snapshot)
			}
			// Reset intervals are uncertain beyond the current segment. Keep the
			// combined amount undated until segment timing is separately resolved.
			if countRule.Settings.CountMode == "reset_total" {
				from = time.Time{}
				until = time.Time{}
				lastPlayed = time.Time{}
			}
			contribution.From = from
			contribution.Until = until
			contribution.Attribution = aggregateAttribution(r, config, from, until, evidence)
			contribution.Rules = append(contribution.Rules, contribution.Attribution.Rules...)
			if !track.Music || r.Values.Title == "" || r.Values.Artist == "" {
				contribution.Reason = "unqualified library track"
				plan.Aggregates = append(plan.Aggregates, contribution)
				continue
			}
			switch contribution.Attribution.Listener {
			case "self":
				contribution.Attributed = amount
			case "partial":
				if contribution.Attribution.Share != nil {
					contribution.Attributed = proportion(amount, *contribution.Attribution.Share)
				}
			default:
				contribution.Reason = "listener: " + contribution.Attribution.Listener
				plan.Aggregates = append(plan.Aggregates, contribution)
				continue
			}
			// An unknown interval cannot establish non-overlap with the observed
			// history. Reserve matching personal evidence conservatively.
			candidates := map[string]plannedPlay{}
			for _, key := range evidenceMusicKeys(r.Values, music) {
				for _, p := range reserved[key] {
					other := byID[p.Representative]
					at := p.At
					if at.IsZero() {
						at = other.Start
					}
					precision := time.Second
					if p.Evidence == "observed" && other.Values.Precision > int64(precision) {
						precision = time.Duration(other.Values.Precision)
					}
					if r.UserID != other.UserID || !musicCompatible(r.Values, other.Values, music) || !from.IsZero() && !at.Add(precision).After(from) || !until.IsZero() && at.After(until) {
						continue
					}
					candidates[p.Key] = p
				}
			}
			contribution.CoveredPlays = []string{}
			candidateKeys := make([]string, 0, len(candidates))
			for key := range candidates {
				candidateKeys = append(candidateKeys, key)
			}
			sort.Strings(candidateKeys)
			// Account for observed evidence before earlier estimates.
			sort.SliceStable(candidateKeys, func(i, j int) bool {
				return candidates[candidateKeys[i]].Evidence == "observed" && candidates[candidateKeys[j]].Evidence != "observed"
			})
			for _, key := range candidateKeys {
				if contribution.Observed+contribution.Overlap >= contribution.Attributed {
					break
				}
				candidate := candidates[key]
				if candidate.Evidence == "observed" {
					if claimedObserved[key] {
						continue
					}
					contribution.Observed++
					claimedObserved[key] = true
				} else {
					// Estimated positions cannot prove that independent cumulative
					// counts describe the same plays. Only an equal reported last
					// endpoint can corroborate an already represented event.
					if candidate.Evidence != "last_played" || !playMatching.Enabled || lastPlayed.IsZero() || !candidate.At.Equal(lastPlayed) {
						continue
					}
					contribution.Overlap++
				}
				contribution.CoveredPlays = append(contribution.CoveredPlays, key)
			}
			remaining := contribution.Attributed - contribution.Observed - contribution.Overlap
			timeRule, timeConflict := chooseRule(config, "inference", "time", func(rule Rule) bool {
				scopeUntil := until
				if !scopeUntil.IsZero() {
					scopeUntil = scopeUntil.Add(time.Nanosecond)
				}
				return rule.Scope.Service == "itunes" && scopeContains(rule.Scope, r, from, scopeUntil)
			})
			if timeRule != nil && !timeConflict {
				contribution.Rules = append(contribution.Rules, timeRule.ID)
			}
			used := map[int64]bool{}
			for _, p := range candidates {
				other := byID[p.Representative]
				at := p.At
				if at.IsZero() {
					at = other.Start
				}
				used[at.Unix()] = true
			}
			type occupiedInterval struct{ from, until time.Time }
			occupied := []occupiedInterval{}
			for _, p := range candidates {
				other := byID[p.Representative]
				at := p.At
				if at.IsZero() {
					at = other.Start
				}
				precision := time.Second
				if p.Evidence == "observed" && other.Values.Precision > int64(precision) {
					precision = time.Duration(other.Values.Precision)
				}
				occupied = append(occupied, occupiedInterval{at, at.Add(precision)})
			}
			sort.Slice(occupied, func(i, j int) bool { return occupied[i].from.Before(occupied[j].from) })
			merged := []occupiedInterval{}
			for _, interval := range occupied {
				if len(merged) > 0 && !interval.from.After(merged[len(merged)-1].until) {
					if interval.until.After(merged[len(merged)-1].until) {
						merged[len(merged)-1].until = interval.until
					}
				} else {
					merged = append(merged, interval)
				}
			}
			occupiedAt := func(at time.Time) bool {
				if used[at.Unix()] {
					return true
				}
				i := sort.Search(len(merged), func(i int) bool { return merged[i].until.After(at) })
				return i < len(merged) && !at.Before(merged[i].from)
			}
			nextAvailable := func(at time.Time) (time.Time, bool) {
				for !at.After(until) {
					i := sort.Search(len(merged), func(i int) bool { return merged[i].until.After(at) })
					if i < len(merged) && !at.Before(merged[i].from) {
						end := merged[i].until
						at = time.Unix(end.Unix(), 0).UTC()
						if at.Before(end) {
							at = at.Add(time.Second)
						}
						continue
					}
					if !used[at.Unix()] {
						return at, true
					}
					at = at.Add(time.Second)
				}
				return time.Time{}, false
			}
			addPlay := func(at time.Time, level string, index int64) {
				p := plannedPlay{Key: digest([]byte(fmt.Sprintf("inference:%s:%s:%d", contribution.Key, level, index))), Representative: r.ID, Records: append([]int64{}, contribution.Records...), Reason: level, At: at, Evidence: level, IntervalStart: from, IntervalEnd: until, Attribution: contribution.Attribution}
				plan.Plays = append(plan.Plays, p)
				plan.Attributions[p.Key] = p.Attribution
				for _, key := range evidenceMusicKeys(r.Values, music) {
					reserved[key] = append(reserved[key], p)
				}
				used[at.Unix()] = true
				remaining--
			}
			if timeRule != nil && !timeConflict && !until.IsZero() && remaining > 0 {
				if timeRule.Settings.UseLastPlayed && contribution.Attribution.Listener == "self" && !lastPlayed.IsZero() && !occupiedAt(lastPlayed) {
					addPlay(lastPlayed, "last_played", 0)
					contribution.LastPlayed++
				}
				if timeRule.Settings.EstimateTimes && !from.IsZero() && until.After(from) {
					span := until.Unix() - from.Unix()
					requested := remaining
					// One million generated entries is a per-rebuild resource bound.
					// Excess remains an exact undated amount with its source evidence.
					limit := requested
					if limit > 1000000-totalEstimates {
						limit = 1000000 - totalEstimates
					}
					for i := int64(1); i <= limit; i++ {
						if err := ctx.Err(); err != nil {
							return err
						}
						denominator := requested
						if denominator == math.MaxInt64 {
							denominator--
						}
						denominator++
						offset := span/denominator*i + (span%denominator)*i/denominator
						at := time.Unix(from.Unix()+offset, 0).UTC()
						at, free := nextAvailable(at)
						if !free {
							break
						}
						addPlay(at, "estimated", i)
						contribution.Estimated++
						totalEstimates++
					}
				}
			}
			contribution.Undated = remaining
			plan.Aggregates = append(plan.Aggregates, contribution)
		}
	}
	return nil
}

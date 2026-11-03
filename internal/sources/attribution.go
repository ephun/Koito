package sources

import (
	"encoding/json"
	"sort"
	"time"
)

type Attribution struct {
	Listener string   `json:"listener"`
	Basis    string   `json:"basis"`
	Rules    []string `json:"rules"`
	Share    *int     `json:"share_percent,omitempty"`
	Rank     int      `json:"-"`
}

func scopeRank(s RuleScope) int {
	rank := 0
	if s.Service != "" {
		rank++
	}
	if s.SourceID != 0 {
		rank += 10
	}
	if s.LibraryID != "" {
		rank += 20
	}
	if s.From != "" {
		rank += 4
	}
	if s.Until != "" {
		rank += 4
	}
	if s.Device != "" {
		rank += 8
	}
	if s.Artist != "" {
		rank += 8
	}
	if s.FileHash != "" {
		rank += 100
	}
	if s.TrackID != "" {
		rank += 200
	}
	if s.RecordID != 0 {
		rank += 10000
	}
	return rank
}

// Scope boundaries contain the whole supporting interval. A partial overlap
// cannot establish attribution for an undated lifetime count.
func scopeContains(s RuleScope, r matchingRecord, from, until time.Time) bool {
	if s.Service != "" && s.Service != r.Service || s.SourceID != 0 && s.SourceID != r.SourceID || s.FileHash != "" && s.FileHash != r.FileHash || s.LibraryID != "" && s.LibraryID != r.LibraryID || s.TrackID != "" && s.TrackID != r.TrackID || s.RecordID != 0 && s.RecordID != r.ID || s.Artist != "" && s.Artist != r.Values.Artist || s.Device != "" && s.Device != r.Device {
		return false
	}
	if s.From != "" {
		date, _ := time.Parse(time.RFC3339Nano, s.From)
		if from.IsZero() || from.Before(date) {
			return false
		}
	}
	if s.Until != "" {
		date, _ := time.Parse(time.RFC3339Nano, s.Until)
		if until.IsZero() || until.After(date) {
			return false
		}
	}
	return true
}

func chooseRule(config RuleConfig, kind, subtype string, applies func(Rule) bool) (*Rule, bool) {
	var selected *Rule
	best := -1
	conflict := false
	for i := range config.Rules {
		r := &config.Rules[i]
		if !r.Enabled || r.Type != kind || r.Subtype != subtype || !applies(*r) {
			continue
		}
		rank := scopeRank(r.Scope)
		if rank > best {
			selected = r
			best = rank
			conflict = false
			continue
		}
		if rank == best {
			a, _ := json.Marshal(selected.Settings)
			b, _ := json.Marshal(r.Settings)
			if string(a) != string(b) {
				conflict = true
			}
			if r.ID < selected.ID {
				selected = r
			}
		}
	}
	return selected, conflict
}

func directAttribution(r matchingRecord, config RuleConfig, from, until time.Time) Attribution {
	rule, conflict := chooseRule(config, "attribution", "direct", func(rule Rule) bool { return scopeContains(rule.Scope, r, from, until) })
	if conflict {
		return Attribution{Listener: "unknown", Basis: "conflicting direct rules", Rules: []string{}, Rank: scopeRank(rule.Scope)}
	}
	if rule != nil {
		return Attribution{Listener: rule.Settings.Listener, Basis: "direct", Rules: []string{rule.ID}, Rank: scopeRank(rule.Scope)}
	}
	if r.Service == "itunes" {
		return Attribution{Listener: "unknown", Basis: "unattributed library", Rules: []string{}}
	}
	return Attribution{Listener: "self", Basis: "existing history", Rules: []string{}}
}

func recordInterval(r matchingRecord) (time.Time, time.Time) {
	if r.Start.IsZero() {
		return time.Time{}, time.Time{}
	}
	precision := time.Duration(r.Values.Precision)
	if precision <= 0 {
		precision = time.Second
	}
	return r.Start, r.Start.Add(precision)
}

func combineAttributions(values []Attribution) Attribution {
	explicitSelf, other, unknown := false, false, false
	conflicting := false
	rules := map[string]bool{}
	strongest := -1
	for _, v := range values {
		if v.Basis != "existing history" && v.Rank > strongest {
			strongest = v.Rank
		}
	}
	for _, v := range values {
		if v.Basis == "existing history" || v.Rank < strongest {
			continue
		}
		for _, id := range v.Rules {
			rules[id] = true
		}
		if v.Basis == "conflicting direct rules" {
			conflicting = true
		}
		switch v.Listener {
		case "self":
			explicitSelf = true
		case "other":
			other = true
		default:
			unknown = true
		}
	}
	out := Attribution{Listener: "self", Basis: "existing history", Rules: []string{}}
	for id := range rules {
		out.Rules = append(out.Rules, id)
	}
	sort.Strings(out.Rules)
	switch {
	case conflicting:
		out.Listener = "unknown"
		out.Basis = "conflicting direct rules"
	case explicitSelf && other:
		out.Listener = "unknown"
		out.Basis = "conflicting listeners"
	case explicitSelf:
		out.Basis = "direct"
	case other:
		out.Listener = "other"
		out.Basis = "direct"
	case unknown:
		out.Listener = "unknown"
		out.Basis = "unknown listener"
	}
	return out
}

func recordIndex(records []matchingRecord) map[int64]matchingRecord {
	out := map[int64]matchingRecord{}
	for _, r := range records {
		out[r.ID] = r
	}
	return out
}

type personalEvidence struct {
	play   plannedPlay
	record matchingRecord
}

func evidenceMusicKeys(v recordValues, rule Rule) []string {
	normalize := func(s string) string {
		if rule.Enabled && !rule.Settings.CaseSensitive {
			return foldedName(s)
		}
		return s
	}
	out := []string{"text:" + normalize(v.Artist) + "\x00" + normalize(v.Title)}
	if v.URI != "" {
		out = append(out, "uri:"+v.URI)
	}
	if v.RecordingID != "" {
		out = append(out, "recording:"+v.RecordingID)
	}
	return out
}

type evidenceIndex map[string][]personalEvidence

func buildPersonalEvidence(plan matchPlan, config RuleConfig) evidenceIndex {
	byID := recordIndex(plan.Records)
	music, _ := config.matchingRules()
	out := evidenceIndex{}
	for _, p := range plan.Plays {
		if p.Attribution.Listener != "self" || p.Attribution.Basis == "heuristic" {
			continue
		}
		for _, id := range p.Records {
			r := byID[id]
			from, until := recordInterval(r)
			a := directAttribution(r, config, from, until)
			if a.Listener != "self" || a.Basis != "direct" {
				continue
			}
			for _, key := range evidenceMusicKeys(r.Values, music) {
				out[key] = append(out[key], personalEvidence{p, r})
			}
		}
	}
	return out
}

func evidenceMatches(index evidenceIndex, r matchingRecord, source int64, minimum int, config RuleConfig, from, until time.Time) bool {
	music, _ := config.matchingRules()
	seen := map[string]bool{}
	for _, key := range evidenceMusicKeys(r.Values, music) {
		for _, e := range index[key] {
			if seen[e.play.Key] || e.record.UserID != r.UserID || e.record.SourceID != source || !musicCompatible(r.Values, e.record.Values, music) {
				continue
			}
			if !from.IsZero() && e.record.Start.Before(from) || !until.IsZero() && e.record.Start.After(until) {
				continue
			}
			seen[e.play.Key] = true
			if len(seen) >= minimum {
				return true
			}
		}
	}
	return false
}

func applyAttribution(plan *matchPlan, config RuleConfig) {
	byID := recordIndex(plan.Records)
	plan.Attributions = map[string]Attribution{}
	for i := range plan.Plays {
		p := &plan.Plays[i]
		values := []Attribution{}
		for _, id := range p.Records {
			r := byID[id]
			from, until := recordInterval(r)
			a := directAttribution(r, config, from, until)
			if a.Basis == "existing history" {
				if rule, _ := chooseRule(config, "attribution", "heuristic", func(rule Rule) bool { return scopeContains(rule.Scope, r, from, until) }); rule != nil {
					a.Listener = "unknown"
					a.Basis = "heuristic pending"
				}
			}
			values = append(values, a)
		}
		p.Attribution = combineAttributions(values)
	}
	evidence := buildPersonalEvidence(*plan, config)
	kept := []plannedPlay{}
	for _, p := range plan.Plays {
		if p.Attribution.Listener == "unknown" && p.Attribution.Basis == "unknown listener" && len(p.Attribution.Rules) == 0 {
			for _, id := range p.Records {
				r := byID[id]
				from, until := recordInterval(r)
				rule, conflict := chooseRule(config, "attribution", "heuristic", func(rule Rule) bool { return scopeContains(rule.Scope, r, from, until) })
				if rule == nil || conflict {
					continue
				}
				var windowFrom, windowUntil time.Time
				if rule.Scope.From != "" {
					windowFrom, _ = time.Parse(time.RFC3339Nano, rule.Scope.From)
				}
				if rule.Scope.Until != "" {
					windowUntil, _ = time.Parse(time.RFC3339Nano, rule.Scope.Until)
				}
				if evidenceMatches(evidence, r, rule.Settings.EvidenceSourceID, rule.Settings.MinimumMatches, config, windowFrom, windowUntil) {
					p.Attribution = Attribution{Listener: "self", Basis: "heuristic", Rules: []string{rule.ID}}
					break
				}
			}
		}
		plan.Attributions[p.Key] = p.Attribution
		if p.Attribution.Listener == "self" {
			p.Evidence = "observed"
			kept = append(kept, p)
		} else {
			for _, id := range p.Records {
				plan.Reasons[id] = "listener: " + p.Attribution.Listener
			}
		}
	}
	plan.Plays = kept
}

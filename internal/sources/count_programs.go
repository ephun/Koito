package sources

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func countSchema() filters.Schema {
	minimum, maximum := int64(0), int64(math.MaxInt64)
	return filters.Schema{
		Help:           map[string]string{"library.matched_repeats": "Navidrome scrobble records matched to an already counted play.", "library.snapshots": "Records for the same track across library exports.", "library.counts": "Valid counts from the selected supporting records.", "library.latest_count": "Read the count from the newest dated export record.", "library.reset_total": "Add each completed counter total when a later record reports a smaller count.", "count.snapshot": "Use this record's metadata when assigning names and dates.", "count.undated": "Use an undated total when the count spans uncertain dates."},
		SnapshotFields: true,
		ItemFields:     map[string]filters.Type{"item:/count": filters.Number, "item:/count_valid": filters.Boolean, "item:/date": filters.Date, "item:/dated": filters.Boolean, "item:/added": filters.Date, "item:/last_played": filters.Date, "item:/file_hash": filters.Text, "item:/artist": filters.Text, "item:/album": filters.Text, "item:/title": filters.Text, "item:/persistent_id": filters.Text, "item:/record_id": filters.Number, "item:/source_id": filters.Number, "item:/source_service": filters.Text, "item:/library_id": filters.Text},
		ItemLabels:     map[string]string{"item:/count": "Reported count", "item:/count_valid": "Count available", "item:/date": "Export date", "item:/dated": "Export date available", "item:/added": "Date added", "item:/last_played": "Last played", "item:/file_hash": "File", "item:/artist": "Original artist", "item:/album": "Original album", "item:/title": "Original track title", "item:/persistent_id": "Persistent track identifier", "item:/record_id": "Record identifier", "item:/source_id": "Source identifier", "item:/source_service": "Source", "item:/library_id": "Library identifier"},
		Fields:         map[string]filters.Type{"library.matched_repeats": filters.Number, "library.snapshots": filters.List, "library.counts": filters.List, "library.id": filters.Text, "library.artist": filters.Text, "library.album": filters.Text, "library.title": filters.Text, "library.maximum_snapshot": filters.Object, "library.latest_snapshot": filters.Object, "library.reset_snapshot": filters.Object, "library.latest_count": filters.Number, "library.reset_total": filters.Number},
		Targets:        map[string]filters.Type{"count.snapshots": filters.List, "count.total": filters.Number, "count.snapshot": filters.Object, "count.undated": filters.Boolean},
		Bounds:         map[string]filters.NumberBounds{"count.total": {Min: &minimum, Max: &maximum, Whole: true}},
		Labels:         map[string]string{"library.matched_repeats": "Matched scrobble duplicates", "library.snapshots": "Library export records", "library.counts": "Reported counts", "library.id": "Library identifier", "library.artist": "Library artist", "library.album": "Library album", "library.title": "Library track title", "library.maximum_snapshot": "Record with the largest count", "library.latest_snapshot": "Latest dated export record", "library.reset_snapshot": "Latest record for reset totals", "library.latest_count": "Latest reported count", "library.reset_total": "Total assuming counter resets", "count.snapshots": "Supporting records", "count.total": "Cumulative count", "count.snapshot": "Record for names and dates", "count.undated": "Keep count undated"},
	}
}

func snapshotScopeExpression(scope RuleScope) filters.Expr {
	expression := scopeExpression(scope)
	fields := map[string]string{"source.service": "source_service", "source.id": "source_id", "file.hash": "file_hash", "record.id": "record_id", "library.id": "library_id", "track.persistent_id": "persistent_id", "original.artist": "artist", "record.device": "device"}
	var rewrite func(filters.Expr) filters.Expr
	rewrite = func(e filters.Expr) filters.Expr {
		if e.Op == "field" {
			if key, ok := fields[e.Field]; ok {
				return fieldExpr("item:/" + key)
			}
		}
		for i := range e.Args {
			e.Args[i] = rewrite(e.Args[i])
		}
		return e
	}
	return rewrite(expression)
}

func countProgram(mode string, scope RuleScope) *filters.Program {
	input := fieldExpr("library.snapshots")
	if scope != (RuleScope{}) {
		input = operation("where", input, snapshotScopeExpression(scope))
	}
	total := operation("max", fieldExpr("library.counts"))
	snapshot := fieldExpr("library.maximum_snapshot")
	if mode == "latest" {
		total = fieldExpr("library.latest_count")
		snapshot = fieldExpr("library.latest_snapshot")
	}
	if mode == "reset_total" {
		total = fieldExpr("library.reset_total")
		snapshot = fieldExpr("library.reset_snapshot")
	}
	condition := operation("gt", operation("count", fieldExpr("count.snapshots")), literalExpr(0))
	total = operation("subtract", total, fieldExpr("library.matched_repeats"))
	actions := []filters.Statement{{Set: "count.snapshot", Value: &snapshot}, {Set: "count.total", Value: &total}}
	if mode == "reset_total" {
		undated := literalExpr(true)
		actions = append(actions, filters.Statement{Set: "count.undated", Value: &undated})
	}
	return &filters.Program{Steps: []filters.Statement{{Set: "count.snapshots", Value: &input}, {If: &condition, Then: actions}}}
}

func migrateCountPrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Program != nil || r.Type != "inference" || r.Subtype != "count" {
			continue
		}
		scope := r.Scope
		priority := scopeRank(scope)
		r.Priority = &priority
		r.Program = countProgram(r.Settings.CountMode, scope)
		r.Migration = migratedProgram(*r)
		r.Migration.Scope = &scope
		r.Scope = RuleScope{}
		if r.Name == "" {
			r.Name = "iTunes cumulative counts"
		}
	}
	return config
}

func prepareCountPrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Program == nil || r.Type != "inference" || r.Subtype != "count" {
			continue
		}
		compiled, err := filters.CompileProgram(*r.Program, countSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
		r.compiled = compiled
	}
	return nil
}

func hasCountPrograms(config RuleConfig) bool {
	for _, r := range config.Rules {
		if r.Enabled && r.Type == "inference" && r.Subtype == "count" && r.Program != nil {
			return true
		}
	}
	return false
}

func retainCountRaw(r *matchingRecord) error {
	root, err := decodeRaw(r.Payload)
	if err != nil {
		return err
	}
	object, ok := root.(map[string]any)
	if !ok || object["kind"] != "library_track" {
		return nil
	}
	if r.RawFields == nil {
		r.RawFields = filters.Env{}
	}
	r.RawFields["raw:"] = rawValue(root, "raw:")
	return nil
}

func snapshotValue(s LibrarySnapshot, r matchingRecord, library LibraryEvidence) filters.Value {
	count := filters.Absent("reported count")
	if s.Count != nil {
		count = filters.Integer(*s.Count)
	}
	fields := map[string]any{"record_id": r.ID, "source_id": r.SourceID, "source_service": r.Service, "file_hash": s.FileHash, "library_id": library.LibraryID, "persistent_id": r.TrackID, "artist": r.Values.Artist, "album": r.Values.Album, "title": r.Values.Title, "device": r.Device, "count": count, "count_valid": s.Count != nil, "date_text": s.Snapshot, "dated": !libraryDate(s.Snapshot).IsZero()}
	for key, text := range map[string]string{"date": s.Snapshot, "added": s.Added, "last_played": s.LastPlayed} {
		if date := libraryDate(text); !date.IsZero() {
			fields[key] = date
		}
	}
	if raw, ok := r.RawFields["raw:"]; ok {
		fields["raw"] = raw
	}
	value, _ := filters.From(fields)
	return value
}

func selectedSnapshots(value filters.Value, all []LibrarySnapshot) ([]LibrarySnapshot, error) {
	items, ok := value.Items()
	if !ok {
		return nil, fmt.Errorf("Supporting records require a list.")
	}
	byID := map[int64]LibrarySnapshot{}
	for _, s := range all {
		byID[s.RecordID] = s
	}
	seen := map[int64]bool{}
	out := []LibrarySnapshot{}
	for _, item := range items {
		id, valid := item.Lookup("record_id").Int64()
		snapshot, exists := byID[id]
		if !valid || !exists {
			return nil, fmt.Errorf("Choose records belonging to this library track.")
		}
		if !seen[id] {
			out = append(out, snapshot)
			seen[id] = true
		}
	}
	return out, nil
}

func countEnvironment(track LibraryTrack, library LibraryEvidence, byID map[int64]matchingRecord, plans ...*matchPlan) filters.Env {
	all := append([]LibrarySnapshot(nil), track.Snapshots...)
	sort.Slice(all, func(i, j int) bool { return all[i].RecordID < all[j].RecordID })
	values := []filters.Value{}
	for _, s := range all {
		values = append(values, snapshotValue(s, byID[s.RecordID], library))
	}
	env := filters.Env{"library.snapshots": filters.Values(values...), "library.id": filters.String(library.LibraryID), "library.artist": filters.String(track.Artist), "library.album": filters.String(track.Album), "library.title": filters.String(track.Title)}
	env["library.matched_repeats"] = filters.Deferred(func(ctx context.Context, current filters.Env) filters.Value {
		return matchedScrobbleRepeats(track, library, byID, current, plans...)
	})
	inputs := func(current filters.Env) ([]LibrarySnapshot, error) {
		if value, exists := current["count.snapshots"]; exists {
			return selectedSnapshots(value, all)
		}
		return all, nil
	}
	env["library.counts"] = filters.Deferred(func(ctx context.Context, current filters.Env) filters.Value {
		snapshots, err := inputs(current)
		if err != nil {
			return filters.Unknown(err.Error())
		}
		counts := []filters.Value{}
		for _, s := range snapshots {
			if s.Count != nil {
				counts = append(counts, filters.Integer(*s.Count))
			}
		}
		return filters.Values(counts...)
	})
	for _, mode := range []string{"maximum", "latest", "reset_total"} {
		mode := mode
		read := func(current filters.Env, object bool) filters.Value {
			snapshots, err := inputs(current)
			if err != nil {
				return filters.Unknown(err.Error())
			}
			amount, selected, valid := interpretedCount(snapshots, mode)
			if !valid {
				return filters.Unknown("These records do not establish this cumulative count.")
			}
			if object {
				return snapshotValue(selected, byID[selected.RecordID], library)
			}
			return filters.Integer(amount)
		}
		name := map[string]string{"maximum": "library.maximum_snapshot", "latest": "library.latest_snapshot", "reset_total": "library.reset_snapshot"}[mode]
		env[name] = filters.Deferred(func(ctx context.Context, current filters.Env) filters.Value { return read(current, true) })
		if mode != "maximum" {
			amountName := map[string]string{"latest": "library.latest_count", "reset_total": "library.reset_total"}[mode]
			env[amountName] = filters.Deferred(func(ctx context.Context, current filters.Env) filters.Value { return read(current, false) })
		}
	}
	return env
}

type countSelection struct {
	Rule                          Rule
	Snapshots                     []LibrarySnapshot
	Amount                        int64
	Selected                      LibrarySnapshot
	Valid, Undated, FileConfirmed bool
	Issues                        []FilterIssue
}

func countSelectionsConflict(left, right countSelection) bool {
	if left.Rule.legacyComparison != "" && right.Rule.legacyComparison != "" {
		return left.Rule.legacyComparison != right.Rule.legacyComparison
	}
	return left.Valid != right.Valid || left.Amount != right.Amount || left.Undated != right.Undated || left.Selected.RecordID != right.Selected.RecordID
}

func chooseCountRule(ctx context.Context, config RuleConfig, library LibraryEvidence, track LibraryTrack, byID map[int64]matchingRecord, plans ...*matchPlan) (*countSelection, bool, error) {
	var selected *countSelection
	best, conflict := -1, false
	for _, original := range config.Rules {
		if !original.Enabled || original.Type != "inference" || original.Subtype != "count" {
			continue
		}
		candidate := countSelection{Rule: original, Snapshots: append([]LibrarySnapshot(nil), track.Snapshots...)}
		if original.Migration != nil && original.Migration.Scope != nil && legacyComparison(original) != "" && original.Migration.Scope.Service != "" {
			if len(track.Snapshots) == 0 || byID[track.Snapshots[0].RecordID].Service != original.Migration.Scope.Service {
				continue
			}
		}
		candidate.Rule.legacyComparison = legacyComparison(original)
		if original.Program == nil {
			candidate.Snapshots = nil
			for _, s := range track.Snapshots {
				r := byID[s.RecordID]
				r.LibraryID = library.LibraryID
				if scopeContains(original.Scope, r, time.Time{}, time.Time{}) {
					candidate.Snapshots = append(candidate.Snapshots, s)
				}
			}
			if len(candidate.Snapshots) == 0 {
				continue
			}
			candidate.Amount, candidate.Selected, candidate.Valid = interpretedCount(candidate.Snapshots, original.Settings.CountMode)
			candidate.Undated = original.Settings.CountMode == "reset_total"
			candidate.FileConfirmed = original.Scope.FileHash != ""
		} else {
			compiled := original.compiled
			if compiled == nil {
				var err error
				compiled, err = filters.CompileProgram(*original.Program, countSchema())
				if err != nil {
					return nil, false, err
				}
			}
			outcome, err := compiled.Run(ctx, countEnvironment(track, library, byID, plans...))
			if err != nil {
				return nil, false, err
			}
			total, hasTotal := outcome.Values["count.total"]
			if !hasTotal && len(outcome.Issues) == 0 {
				continue
			}
			for _, issue := range outcome.Issues {
				candidate.Issues = append(candidate.Issues, FilterIssue{original.ID, original.Name, issue})
			}
			if inputs, exists := outcome.Values["count.snapshots"]; exists {
				candidate.Snapshots, err = selectedSnapshots(inputs, track.Snapshots)
				if err != nil {
					candidate.Issues = append(candidate.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: "count.snapshots", Reason: err.Error()}})
				}
			}
			if original.Migration != nil && original.Migration.Scope != nil && legacyComparison(original) != "" {
				candidate.FileConfirmed = original.Migration.Scope.FileHash != ""
				if len(outcome.Issues) > 0 {
					candidate.Snapshots = nil
					for _, s := range track.Snapshots {
						r := byID[s.RecordID]
						r.LibraryID = library.LibraryID
						if scopeContains(*original.Migration.Scope, r, time.Time{}, time.Time{}) {
							candidate.Snapshots = append(candidate.Snapshots, s)
						}
					}
				}
			} else if _, explicit := outcome.Values["count.snapshots"]; explicit && len(candidate.Snapshots) > 0 {
				candidate.FileConfirmed = true
				first := candidate.Snapshots[0].FileHash
				for _, s := range candidate.Snapshots {
					if s.FileHash != first {
						candidate.FileConfirmed = false
					}
				}
			}
			candidate.Amount, candidate.Valid = total.Int64()
			candidate.Valid = candidate.Valid && candidate.Amount >= 0 && len(candidate.Issues) == 0
			candidate.Undated = true
			if value, exists := outcome.Values["count.snapshot"]; exists {
				id, valid := value.Lookup("record_id").Int64()
				found := false
				for _, s := range candidate.Snapshots {
					if valid && s.RecordID == id {
						candidate.Selected = s
						candidate.Undated = false
						found = true
						break
					}
				}
				if !found {
					candidate.Valid = false
					candidate.Issues = append(candidate.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: "count.snapshot", Reason: "Choose a record from the supporting records."}})
				}
			}
			if value, exists := outcome.Values["count.undated"]; exists {
				if undated, valid := value.Bool(); valid && undated {
					candidate.Undated = true
				}
			}
			if len(candidate.Snapshots) == 0 {
				candidate.Valid = false
			}
			if candidate.Selected.RecordID == 0 && len(candidate.Snapshots) > 0 {
				candidate.Selected = candidate.Snapshots[0]
			}
		}
		rank := rulePriority(original)
		if rank > best {
			copy := candidate
			selected, best, conflict = &copy, rank, false
		} else if rank == best {
			if countSelectionsConflict(*selected, candidate) {
				conflict = true
			}
			if candidate.Rule.ID < selected.Rule.ID {
				copy := candidate
				selected = &copy
			}
		}
	}
	return selected, conflict, nil
}

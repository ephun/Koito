package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func defaultMusicProgram(sensitive, missing bool) *filters.Program {
	condition := literalExpr(true)
	steps := []filters.Statement{}
	for _, field := range []string{"artist", "album", "title"} {
		value := fieldExpr("original." + field)
		if !sensitive {
			value = operation("casefold", value)
		}
		steps = append(steps, filters.Statement{Set: "music." + field, Value: &value})
	}
	value := literalExpr(missing)
	steps = append(steps, filters.Statement{Set: "music.missing_album", Value: &value})
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: steps}}}
}

func musicSchema() filters.Schema { return music.MatchingSchema() }

func migrateMusicPrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "matching" || r.Subtype != "music" || r.Program != nil {
			continue
		}
		r.Program = defaultMusicProgram(r.Settings.CaseSensitive, r.Settings.MatchMissingAlbum)
		r.Migration = migratedProgram(*r)
		if r.Name == "" {
			r.Name = "Music matching"
		}
	}
	return config
}

func compileMusicPrograms(config RuleConfig) (music.IdentityPrograms, error) {
	rules := []music.IdentityFilter{}
	for _, r := range config.Rules {
		rules = append(rules, music.IdentityFilter{ID: r.ID, Name: r.Name, Type: r.Type, Subtype: r.Subtype, Enabled: r.Enabled, Priority: r.Priority, Program: r.Program, Settings: &music.IdentitySettings{CaseSensitive: r.Settings.CaseSensitive, MatchMissingAlbum: r.Settings.MatchMissingAlbum}})
	}
	return music.CompileIdentityPrograms(rules)
}

func applyMusicPrograms(ctx context.Context, r *matchingRecord, programs music.IdentityPrograms) error {
	if !programs.Active() {
		return nil
	}
	value, issues, err := programs.Resolve(ctx, recordEnvironment(*r), music.MatchIdentity{Artist: r.Values.Artist, Album: r.Values.Album, Title: r.Values.Title})
	if err != nil {
		return err
	}
	r.Values.Music = &value
	for _, issue := range issues {
		r.MusicIssues = append(r.MusicIssues, FilterIssue{issue.ID, issue.Name, issue.Issue})
	}
	return nil
}

func historyComparisonKey(r matchingRecord) string {
	if r.Values.Music == nil {
		return ""
	}
	value := r.Values.Music
	key, _ := json.Marshal([]string{"matching", value.Artist, value.Title, value.Album})
	return string(key)
}

func previewMusic(ctx context.Context, reader sqlReader, plan matchPlan, before, after RuleConfig) ([]NamingExample, error) {
	selectRules := func(config RuleConfig) []Rule {
		rules := []Rule{}
		for _, rule := range config.Rules {
			if rule.Type == "matching" && rule.Subtype == "music" {
				rules = append(rules, rule)
			}
		}
		return rules
	}
	examples := []NamingExample{}
	if jsonHash(selectRules(before)) == jsonHash(selectRules(after)) {
		return examples, nil
	}
	previous, err := loadMatchingRecords(ctx, reader, before, after)
	if err != nil {
		return nil, err
	}
	byID := recordIndex(previous)
	oldRule, _ := before.matchingRules()
	newRule, _ := after.matchingRules()
	seen := map[[3]string]bool{}
	for _, r := range plan.Records {
		old := comparisonMusic(byID[r.ID].Values, oldRule)
		next := comparisonMusic(r.Values, newRule)
		for _, change := range [][3]string{{"Artist", old.Artist, next.Artist}, {"Album", old.Album, next.Album}, {"Track", old.Title, next.Title}, {"Match an empty album", strconv.FormatBool(old.MissingAlbum), strconv.FormatBool(next.MissingAlbum)}} {
			if change[1] != change[2] && !seen[change] {
				seen[change] = true
				examples = append(examples, NamingExample{r.ID, change[0], change[1], change[2]})
				if len(examples) == 12 {
					return examples, nil
				}
			}
		}
	}
	return examples, nil
}

func previewCatalogMusic(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	old, err := compileMusicPrograms(before)
	if err != nil {
		return nil, err
	}
	next, err := compileMusicPrograms(after)
	if err != nil {
		return nil, err
	}
	old, next = old.Catalog(), next.Catalog()
	result := []NamingExample{}
	if !old.Active() && !next.Active() {
		return result, nil
	}
	items, err := catalogNameItems(ctx, tx)
	if err != nil {
		return nil, err
	}
	seen := map[[3]string]bool{}
	for _, item := range items {
		env := filters.Env{"catalog.kind": filters.String(item.kind), "catalog.id": filters.Integer(item.id), "source.service": filters.String("koito"), "original.artist": filters.String(""), "original.album": filters.String(""), "original.title": filters.String("")}
		for key, value := range item.metadata {
			env[key] = value
		}
		field := map[string]string{"artist": "artist", "album": "album", "track": "title"}[item.kind]
		env["original."+field] = filters.String(item.name)
		original := music.MatchIdentity{}
		original.Artist, _ = env["original.artist"].Text()
		original.Album, _ = env["original.album"].Text()
		original.Title, _ = env["original.title"].Text()
		a, _, err := old.Resolve(ctx, env, original)
		if err != nil {
			return nil, err
		}
		b, _, err := next.Resolve(ctx, env, original)
		if err != nil {
			return nil, err
		}
		for _, change := range [][3]string{{"Artist comparison", a.Artist, b.Artist}, {"Album comparison", a.Album, b.Album}, {"Track comparison", a.Title, b.Title}} {
			if change[1] != change[2] && !seen[change] {
				seen[change] = true
				result = append(result, NamingExample{Field: change[0] + ": " + item.name, Before: change[1], After: change[2]})
				if len(result) == 12 {
					return result, nil
				}
			}
		}
	}
	return result, nil
}

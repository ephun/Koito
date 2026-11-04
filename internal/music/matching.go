package music

import (
	"context"
	"fmt"
	"sort"

	"github.com/gabehf/koito/internal/filters"
)

type MatchIdentity struct {
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Title        string `json:"title"`
	MissingAlbum bool   `json:"missing_album"`
}

type IdentityFilter struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Type     string           `json:"type"`
	Subtype  string           `json:"subtype"`
	Enabled  bool             `json:"enabled"`
	Priority *int             `json:"priority,omitempty"`
	Program  *filters.Program `json:"program,omitempty"`
}

type IdentityIssue struct {
	ID   string
	Name string
	filters.Issue
}
type identityProgram struct {
	rule     IdentityFilter
	compiled *filters.CompiledProgram
}
type IdentityPrograms struct{ programs []identityProgram }

func MatchingSchema() filters.Schema {
	return filters.Schema{RawFields: true, Fields: map[string]filters.Type{
		"catalog.kind": filters.Text, "catalog.id": filters.Number, "original.identifier": filters.Text, "original.artist": filters.Text, "original.album": filters.Text, "original.title": filters.Text,
		"source.service": filters.Text, "source.id": filters.Number, "file.hash": filters.Text, "record.id": filters.Number, "record.time": filters.Date, "record.device": filters.Text, "library.id": filters.Text, "track.persistent_id": filters.Text, "track.spotify_uri": filters.Text, "track.recording_id": filters.Text,
	}, Targets: map[string]filters.Type{"music.identifier": filters.Text, "music.artist": filters.Text, "music.album": filters.Text, "music.title": filters.Text, "music.missing_album": filters.Boolean}, Labels: map[string]string{"catalog.kind": "Item type", "catalog.id": "Item identifier", "original.identifier": "Original MusicBrainz identifier", "music.identifier": "MusicBrainz identifier", "music.artist": "Artist used for matching", "music.album": "Album used for matching", "music.title": "Track title used for matching", "music.missing_album": "Match an empty album"}, Help: map[string]string{"music.identifier": "Choose a MusicBrainz identifier for this item. Empty text clears its displayed identifier.", "music.artist": "Choose a comparison name while keeping original and displayed artist names separate.", "music.title": "Choose a comparison title while keeping original and displayed track titles separate.", "music.album": "Choose a comparison album while retaining each play's reported album.", "music.missing_album": "Allow this record's empty album to match a supplied album."}}
}

func CompileIdentityPrograms(rules []IdentityFilter) (IdentityPrograms, error) {
	programs := IdentityPrograms{}
	for _, rule := range rules {
		if !rule.Enabled || rule.Type != "matching" || rule.Subtype != "music" || rule.Program == nil {
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, MatchingSchema())
		if err != nil {
			return programs, fmt.Errorf("Filter %s: %w", rule.ID, err)
		}
		programs.programs = append(programs.programs, identityProgram{rule, compiled})
	}
	sort.SliceStable(programs.programs, func(i, j int) bool {
		a, b := 0, 0
		if p := programs.programs[i].rule.Priority; p != nil {
			a = *p
		}
		if p := programs.programs[j].rule.Priority; p != nil {
			b = *p
		}
		return a < b
	})
	return programs, nil
}

func (p IdentityPrograms) Resolve(ctx context.Context, env filters.Env, original MatchIdentity) (MatchIdentity, []IdentityIssue, error) {
	current := original
	issues := []IdentityIssue{}
	local := filters.Env{}
	for key, value := range env {
		local[key] = value
	}
	for _, program := range p.programs {
		local["music.artist"] = filters.String(current.Artist)
		local["music.album"] = filters.String(current.Album)
		local["music.title"] = filters.String(current.Title)
		local["music.missing_album"] = filters.Bool(current.MissingAlbum)
		outcome, err := program.compiled.Run(ctx, local)
		if err != nil {
			return current, issues, err
		}
		for _, issue := range outcome.Issues {
			issues = append(issues, IdentityIssue{program.rule.ID, program.rule.Name, issue})
		}
		if len(outcome.Issues) > 0 {
			continue
		}
		if _, assigned := outcome.Values["music.identifier"]; assigned {
			issues = append(issues, IdentityIssue{program.rule.ID, program.rule.Name, filters.Issue{Path: "music.identifier", Reason: "Catalog identifiers require an artist, album, or track context."}})
			continue
		}
		next := current
		for key, value := range outcome.Values {
			switch key {
			case "music.artist":
				next.Artist, _ = value.Text()
			case "music.album":
				next.Album, _ = value.Text()
			case "music.title":
				next.Title, _ = value.Text()
			case "music.missing_album":
				next.MissingAlbum, _ = value.Bool()
			}
		}
		if original.Artist != "" && next.Artist == "" || original.Title != "" && next.Title == "" {
			issues = append(issues, IdentityIssue{program.rule.ID, program.rule.Name, filters.Issue{Path: "music", Reason: "Comparison names require an artist and track title."}})
			continue
		}
		current = next
	}
	return current, issues, nil
}

func (p IdentityPrograms) Active() bool { return len(p.programs) > 0 }

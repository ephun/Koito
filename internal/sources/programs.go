package sources

import (
	"context"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

// The editor receives its supported operators and writable fields from the
// same registry used for validation and execution.
type ProgramSchema struct {
	Additions []Rule                       `json:"additions,omitempty"`
	Templates map[string][]ProgramTemplate `json:"templates,omitempty"`
	Legacy    map[string]Rule              `json:"legacy,omitempty"`
	Operators []filters.Operator           `json:"operators"`
	Schemas   map[string]filters.Schema    `json:"schemas"`
}

type ProgramTemplate struct {
	Name    string           `json:"name"`
	Label   string           `json:"label"`
	Program *filters.Program `json:"program"`
}

func namingSchema() filters.Schema {
	return filters.Schema{
		RawFields: true,
		Fields: map[string]filters.Type{
			"original.artist": filters.Text, "original.album": filters.Text, "original.title": filters.Text,
			"source.service": filters.Text, "source.id": filters.Number, "file.hash": filters.Text,
			"record.id": filters.Number, "record.time": filters.Date, "record.device": filters.Text,
			"library.id": filters.Text, "track.persistent_id": filters.Text, "track.spotify_uri": filters.Text, "track.recording_id": filters.Text,
		},
		Targets: map[string]filters.Type{"display.artist": filters.Text, "display.album": filters.Text, "display.title": filters.Text},
	}
}

type namingProgram struct {
	rule    Rule
	program *filters.CompiledProgram
}

type NamingExample struct {
	RecordID int64  `json:"record_id"`
	Field    string `json:"field"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

func previewNaming(ctx context.Context, plan matchPlan, before, after RuleConfig) ([]NamingExample, error) {
	oldPrograms, err := compileNaming(before)
	if err != nil {
		return nil, err
	}
	newPrograms, err := compileNaming(after)
	if err != nil {
		return nil, err
	}
	examples := []NamingExample{}
	if len(oldPrograms) == 0 && len(newPrograms) == 0 {
		return examples, nil
	}
	records := map[int64]matchingRecord{}
	for _, r := range plan.Records {
		records[r.ID] = r
	}
	seen := map[int64]bool{}
	shown := map[[3]string]bool{}
	ids := []int64{}
	for _, p := range plan.Plays {
		ids = append(ids, p.Representative)
	}
	for _, a := range plan.Aggregates {
		if a.Attributed > 0 {
			ids = append(ids, a.Representative)
		}
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		r := records[id]
		v := spotifyItem{Title: r.Values.Title, Artist: r.Values.Artist, Album: r.Values.Album, URI: r.Values.URI, RecordingID: r.Values.RecordingID}
		old, err := applyNaming(ctx, oldPrograms, r, v)
		if err != nil {
			return nil, err
		}
		next, err := applyNaming(ctx, newPrograms, r, v)
		if err != nil {
			return nil, err
		}
		for _, change := range [][3]string{{"Artist", old.Artist, next.Artist}, {"Album", old.Album, next.Album}, {"Track", old.Title, next.Title}} {
			if change[1] == change[2] || shown[change] {
				continue
			}
			shown[change] = true
			examples = append(examples, NamingExample{id, change[0], change[1], change[2]})
			if len(examples) == 12 {
				return examples, nil
			}
		}
	}
	return examples, nil
}

func compileNaming(config RuleConfig) ([]namingProgram, error) {
	var programs []namingProgram
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "presentation" || rule.Subtype != "naming" {
			continue
		}
		if rule.Program == nil {
			return nil, fmt.Errorf("Naming filter %s has no program.", rule.ID)
		}
		compiled, err := filters.CompileProgram(*rule.Program, namingSchema())
		if err != nil {
			return nil, err
		}
		programs = append(programs, namingProgram{rule, compiled})
	}
	return programs, nil
}

func recordEnvironment(r matchingRecord) filters.Env {
	env := filters.Env{
		"original.artist": filters.String(r.Values.Artist), "original.album": filters.String(r.Values.Album), "original.title": filters.String(r.Values.Title),
		"source.service": filters.String(r.Service), "source.id": filters.Integer(r.SourceID), "file.hash": filters.String(r.FileHash), "record.id": filters.Integer(r.ID),
		"display.artist": filters.String(r.Values.Artist), "display.album": filters.String(r.Values.Album), "display.title": filters.String(r.Values.Title),
	}
	for key, value := range r.RawFields {
		env[key] = value
	}
	if r.Values.Music != nil {
		value := r.Values.Music
		env["music.artist"] = filters.String(value.Artist)
		env["music.album"] = filters.String(value.Album)
		env["music.title"] = filters.String(value.Title)
		env["music.missing_album"] = filters.Bool(value.MissingAlbum)
	}
	if !r.Start.IsZero() {
		env["record.time"] = filters.Timestamp(r.Start)
	}
	for key, value := range map[string]string{"record.device": r.Device, "library.id": r.LibraryID, "track.persistent_id": r.TrackID, "track.spotify_uri": r.Values.URI, "track.recording_id": r.Values.RecordingID} {
		if value != "" {
			env[key] = filters.String(value)
		}
	}
	return env
}

func applyNaming(ctx context.Context, programs []namingProgram, r matchingRecord, v spotifyItem) (spotifyItem, error) {
	if len(programs) == 0 {
		return v, nil
	}
	env := recordEnvironment(r)
	for _, program := range programs {
		from, until := recordInterval(r)
		if !scopeContains(program.rule.Scope, r, from, until) {
			continue
		}
		result, err := program.program.Run(ctx, env)
		if err != nil {
			return v, err
		}
		if len(result.Issues) > 0 {
			continue
		}
		// Validate the complete change before allowing later filters to read it.
		candidate := v
		for key, value := range result.Values {
			text, ok := value.Text()
			if !ok {
				return v, fmt.Errorf("Naming filter %s must produce text.", program.rule.ID)
			}
			switch key {
			case "display.artist":
				candidate.Artist = text
			case "display.album":
				candidate.Album = text
			case "display.title":
				candidate.Title = text
			}
		}
		if candidate.Artist == "" || candidate.Title == "" {
			return v, fmt.Errorf("Naming filter %s produced an empty artist or title.", program.rule.ID)
		}
		v = candidate
		for key, value := range result.Values {
			env[key] = value
		}
	}
	return v, nil
}

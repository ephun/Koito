package sources

import (
	"context"
	"encoding/json"
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
			"original.aliases": filters.List, "original.primary_artist_ids": filters.List, "original.artists": filters.List, "catalog.artist_ids": filters.List, "catalog.kind": filters.Text, "catalog.id": filters.Number, "catalog.artist_id": filters.Number, "source.genres": filters.List, "source.genre_records": filters.List,
			"original.artist": filters.Text, "original.album": filters.Text, "original.title": filters.Text,
			"source.service": filters.Text, "source.id": filters.Number, "file.hash": filters.Text,
			"record.id": filters.Number, "record.time": filters.Date, "record.device": filters.Text,
			"library.id": filters.Text, "track.persistent_id": filters.Text, "track.spotify_uri": filters.Text, "track.recording_id": filters.Text,
		},
		Targets: map[string]filters.Type{"display.aliases": filters.List, "display.artist_ids": filters.List, "display.primary_artist_ids": filters.List, "display.artist": filters.Text, "display.album": filters.Text, "display.title": filters.Text, "artist.genres": filters.List, "display.tags": filters.List},
		Help:    map[string]string{"display.aliases": "Choose alternate names used by search.", "display.artist_ids": "Search for credited artists or build a list from expressions.", "display.primary_artist_ids": "Choose primary artists from the assigned credits.", "display.tags": "Choose concise labels from source fields or expressions.", "artist.genres": "Choose artist labels from source evidence or a list. Use a separate filter for record names.", "source.genre_records": "Each item includes its label, source identifier, file hash, record identifier, and original field."},
		Labels:  map[string]string{"display.aliases": "Aliases", "original.aliases": "Original aliases", "display.artist_ids": "Artist credits", "display.primary_artist_ids": "Primary artist credits", "original.primary_artist_ids": "Original primary artist identifiers", "original.artists": "Original artist credits", "catalog.artist_ids": "Artist identifiers", "catalog.kind": "Item type", "catalog.id": "Item identifier", "artist.genres": "Artist genres", "display.tags": "Listening tags", "source.genres": "Source genre labels", "source.genre_records": "Source genre records", "catalog.artist_id": "Artist identifier"},
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
		oldTags, _ := json.Marshal(append([]string{}, old.Tags...))
		newTags, _ := json.Marshal(append([]string{}, next.Tags...))
		for _, change := range [][3]string{{"Artist", old.Artist, next.Artist}, {"Album", old.Album, next.Album}, {"Track", old.Title, next.Title}, {"Listening tags", string(oldTags), string(newTags)}} {
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

func namingTargets(program *filters.Program) (names, genres bool) {
	if program == nil {
		return
	}
	var visit func([]filters.Statement)
	visit = func(steps []filters.Statement) {
		for _, step := range steps {
			if step.Set == "artist.genres" {
				genres = true
			} else if step.Set != "" {
				names = true
			}
			visit(step.Then)
			visit(step.Else)
			for _, branch := range step.ElseIf {
				visit(branch.Then)
			}
		}
	}
	visit(program.Steps)
	return
}
func compileNaming(config RuleConfig) ([]namingProgram, error) {
	return compileNamingDomain(config, false)
}
func compileNamingDomain(config RuleConfig, genres bool) ([]namingProgram, error) {
	var programs []namingProgram
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "presentation" || rule.Subtype != "naming" {
			continue
		}
		if rule.Program == nil {
			return nil, fmt.Errorf("Naming filter %s has no program.", rule.ID)
		}
		hasNames, hasGenres := namingTargets(rule.Program)
		if hasGenres && rule.Scope != (RuleScope{}) {
			return nil, fmt.Errorf("Filter %s needs genre conditions inside IF or WHERE.", rule.ID)
		}
		if hasNames && hasGenres {
			return nil, fmt.Errorf("Filter %s must separate artist genres from record naming actions.", rule.ID)
		}
		if hasGenres != genres {
			continue
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
		"catalog.kind":    filters.String("record"),
		"original.artist": filters.String(r.Values.Artist), "original.album": filters.String(r.Values.Album), "original.title": filters.String(r.Values.Title),
		"source.service": filters.String(r.Service), "source.id": filters.Integer(r.SourceID), "file.hash": filters.String(r.FileHash), "record.id": filters.Integer(r.ID),
		"display.artist": filters.String(r.Values.Artist), "display.album": filters.String(r.Values.Album), "display.title": filters.String(r.Values.Title),
	}
	if r.Values.Artist != "" {
		env["original.artists"] = filters.Values(filters.String(r.Values.Artist))
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
	tagValues := []filters.Value{}
	for _, tag := range v.Tags {
		tagValues = append(tagValues, filters.String(tag))
	}
	env["display.tags"] = filters.Values(tagValues...)
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
			if key == "display.tags" {
				items, ok := value.Items()
				if !ok {
					return v, fmt.Errorf("Listening tags require a list of text labels.")
				}
				tags := []string{}
				for _, item := range items {
					text, valid := item.Text()
					if !valid || text == "" {
						return v, fmt.Errorf("Listening tags require nonempty text labels.")
					}
					tags = append(tags, text)
				}
				candidate.Tags = tags
				continue
			}
			if key == "artist.genres" {
				continue
			}
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

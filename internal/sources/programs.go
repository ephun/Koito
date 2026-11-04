package sources

import (
	"context"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

// The editor receives its supported operators and writable fields from the
// same registry used for validation and execution.
type ProgramSchema struct {
	Operators []filters.Operator        `json:"operators"`
	Schemas   map[string]filters.Schema `json:"schemas"`
}

func namingSchema() filters.Schema {
	return filters.Schema{
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

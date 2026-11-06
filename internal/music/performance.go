package music

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/gabehf/koito/internal/filters"
)

func PerformanceFields(s filters.Schema) filters.Schema {
	if s.Labels == nil {
		s.Labels = map[string]string{}
	}
	for field, value := range map[string]struct {
		kind  filters.Type
		label string
	}{
		"event.kind":               {filters.Text, "Listening context"},
		"event.id":                 {filters.Text, "Performance identifier"},
		"event.date_only":          {filters.Boolean, "Only the date is known"},
		"event.time_estimated":     {filters.Boolean, "Estimated performance time"},
		"concert.id":               {filters.Text, "Setlist identifier"},
		"concert.revision":         {filters.Text, "Setlist revision"},
		"concert.date":             {filters.Text, "Concert local date"},
		"concert.venue":            {filters.Text, "Concert venue"},
		"concert.performer":        {filters.Text, "Performing artist"},
		"concert.cover_artist":     {filters.Text, "Original artist of a cover"},
		"concert.set":              {filters.Number, "Set number"},
		"concert.position":         {filters.Number, "Song position"},
		"concert.encore":           {filters.Number, "Encore number"},
		"concert.tape":             {filters.Boolean, "Recorded tape playback"},
		"concert.attended":         {filters.Boolean, "Reported attendance"},
		"concert.legacy":           {filters.Boolean, "Recovered concert record"},
		"game.chart_id":            {filters.Text, "Game chart checksum"},
		"game.player_ids":          {filters.List, "Game player identifiers"},
		"game.player_names":        {filters.List, "Game player names"},
		"game.scores":              {filters.List, "Game player scores"},
		"game.with_replay":         {filters.Boolean, "Played with a replay"},
		"game.has_bots":            {filters.Boolean, "Game includes bots"},
		"game.speed":               {filters.Number, "Game song speed"},
		"game.catalog.album":       {filters.Text, "Game catalog album"},
		"game.catalog.duration_ms": {filters.Number, "Game catalog duration in milliseconds"},
		"game.catalog.records":     {filters.List, "Game catalog record identifiers"},
		"game.catalog.items":       {filters.List, "Game catalog matches"},
	} {
		s.Fields[field] = value.kind
		s.Labels[field] = value.label
	}
	return s
}

type Performance struct {
	Catalog       *GameCatalog `json:"catalog,omitempty"`
	Kind          string       `json:"kind"`
	ID            string       `json:"id"`
	ChartID       string       `json:"chart_id,omitempty"`
	PlayerIDs     []string     `json:"player_ids,omitempty"`
	PlayerNames   []string     `json:"player_names,omitempty"`
	Scores        []any        `json:"scores,omitempty"`
	WithReplay    *bool        `json:"with_replay,omitempty"`
	HasBots       *bool        `json:"has_bots,omitempty"`
	Speed         *float64     `json:"speed,omitempty"`
	SetlistID     string       `json:"setlist_id,omitempty"`
	Revision      string       `json:"revision,omitempty"`
	RevisionHash  string       `json:"revision_hash,omitempty"`
	Updated       string       `json:"updated,omitempty"`
	LocalDate     string       `json:"local_date,omitempty"`
	DateOnly      bool         `json:"date_only,omitempty"`
	TimeEstimated bool         `json:"time_estimated,omitempty"`
	Venue         string       `json:"venue,omitempty"`
	Performer     string       `json:"performer,omitempty"`
	CoverArtist   string       `json:"cover_artist,omitempty"`
	Set           int          `json:"set,omitempty"`
	Position      int          `json:"position,omitempty"`
	Encore        int          `json:"encore,omitempty"`
	Tape          bool         `json:"tape,omitempty"`
	Attended      *bool        `json:"attended,omitempty"`
	Legacy        bool         `json:"legacy,omitempty"`
}

type GameCatalog struct {
	Album      string  `json:"album,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Records    []int64 `json:"records"`
	Items      []any   `json:"items"`
}

func (p *Performance) Environment() filters.Env {
	env := filters.Env{}
	if p == nil {
		return filters.Env{"event.kind": filters.String("unknown"), "event.time_estimated": filters.Bool(false)}
	}
	env["event.kind"], env["event.id"] = filters.String(p.Kind), filters.String(p.ID)
	env["event.date_only"] = filters.Bool(p.DateOnly)
	env["event.time_estimated"] = filters.Bool(p.TimeEstimated)
	if p.Kind == "concert" {
		for field, value := range map[string]string{"concert.id": p.SetlistID, "concert.revision": p.Revision, "concert.date": p.LocalDate, "concert.venue": p.Venue, "concert.performer": p.Performer, "concert.cover_artist": p.CoverArtist} {
			if value != "" {
				env[field] = filters.String(value)
			}
		}
		if p.Set > 0 {
			env["concert.set"], env["concert.encore"] = filters.Integer(int64(p.Set)), filters.Integer(int64(p.Encore))
		}
		if p.Position > 0 {
			env["concert.position"] = filters.Integer(int64(p.Position))
		}
		if p.SetlistID != "" && !p.Legacy {
			env["concert.tape"] = filters.Bool(p.Tape)
		}
		env["concert.legacy"] = filters.Bool(p.Legacy)
		if p.Attended != nil {
			env["concert.attended"] = filters.Bool(*p.Attended)
		}
		return env
	}
	if p.Kind != "gameplay" {
		return env
	}
	env["game.chart_id"] = filters.String(p.ChartID)
	if p.Catalog != nil {
		if p.Catalog.Album != "" {
			env["game.catalog.album"] = filters.String(p.Catalog.Album)
		}
		if p.Catalog.DurationMS != nil {
			env["game.catalog.duration_ms"] = filters.Integer(*p.Catalog.DurationMS)
		}
		ids := []filters.Value{}
		for _, id := range p.Catalog.Records {
			ids = append(ids, filters.Integer(id))
		}
		env["game.catalog.records"] = filters.Values(ids...)
		if value, err := filters.From(p.Catalog.Items); err == nil {
			env["game.catalog.items"] = value
		}
	}
	for field, values := range map[string][]string{"game.player_ids": p.PlayerIDs, "game.player_names": p.PlayerNames} {
		items := []filters.Value{}
		for _, value := range values {
			items = append(items, filters.String(value))
		}
		env[field] = filters.Values(items...)
	}
	if scores, err := filters.From(p.Scores); err == nil {
		env["game.scores"] = scores
	}
	for field, value := range map[string]*bool{"game.with_replay": p.WithReplay, "game.has_bots": p.HasBots} {
		if value != nil {
			env[field] = filters.Bool(*value)
		}
	}
	if p.Speed != nil {
		if value, err := filters.From(*p.Speed); err == nil {
			env["game.speed"] = value
		}
	}
	return env
}

func ReadPerformance(raw string) (*Performance, error) {
	var p Performance
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.UseNumber()
	if err := d.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func LoadPerformances(ctx context.Context, db Database, keys []string) (map[string]*Performance, error) {
	out := map[string]*Performance{}
	for start := 0; start < len(keys); start += 400 {
		end := start + 400
		if end > len(keys) {
			end = len(keys)
		}
		args := []any{}
		for _, key := range keys[start:end] {
			args = append(args, key)
		}
		rows, err := db.QueryContext(ctx, `SELECT play_key,context_json FROM source_play_context WHERE play_key IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key, raw string
			if err = rows.Scan(&key, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			p, err := ReadPerformance(raw)
			if err != nil {
				rows.Close()
				return nil, err
			}
			out[key] = p
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gabehf/koito/internal/filters"
)

type SourceField struct {
	Path  string       `json:"path"`
	Type  filters.Type `json:"type"`
	Label string       `json:"label"`
}

func pointerPart(part string) string {
	return strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
}
func decodePart(part string) string {
	return strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
}

func decodeRaw(payload string) (any, error) {
	d := json.NewDecoder(strings.NewReader(payload))
	d.UseNumber()
	var v any
	err := d.Decode(&v)
	return v, err
}

func rawValue(root any, field string) filters.Value {
	if !filters.ValidRawField(field) {
		return filters.Unknown("Invalid source field.")
	}
	path := strings.TrimPrefix(field, "raw:")
	if strings.HasPrefix(field, "column:") {
		path = strings.TrimPrefix(field, "column:")
		object, ok := root.(map[string]any)
		if !ok {
			return filters.Absent(field)
		}
		columns, ok := object["columns"].([]any)
		if !ok {
			return filters.Absent(field)
		}
		values, ok := object["values"].([]any)
		if !ok {
			return filters.Absent(field)
		}
		name := decodePart(strings.TrimPrefix(path, "/"))
		for i, column := range columns {
			if column == name && i < len(values) {
				value, err := filters.From(values[i])
				if err != nil {
					return filters.Unknown(err.Error())
				}
				return value
			}
		}
		return filters.Absent(field)
	}
	current := root
	if path != "" {
		for _, part := range strings.Split(path[1:], "/") {
			key := decodePart(part)
			switch value := current.(type) {
			case map[string]any:
				var exists bool
				current, exists = value[key]
				if !exists {
					return filters.Absent(field)
				}
			case []any:
				index, err := strconv.Atoi(key)
				if err != nil || index < 0 || index >= len(value) {
					return filters.Absent(field)
				}
				current = value[index]
			default:
				return filters.Absent(field)
			}
		}
	}
	value, err := filters.From(current)
	if err != nil {
		return filters.Unknown(err.Error())
	}
	return value
}

func requestedRawFields(config RuleConfig) []string {
	seen := map[string]bool{}
	var expression func(filters.Expr)
	expression = func(e filters.Expr) {
		if e.Op == "field" && filters.ValidRawField(e.Field) {
			seen[e.Field] = true
		}
		for _, arg := range e.Args {
			expression(arg)
		}
	}
	var statements func([]filters.Statement)
	statements = func(steps []filters.Statement) {
		for _, step := range steps {
			if step.If != nil {
				expression(*step.If)
			}
			if step.Value != nil {
				expression(*step.Value)
			}
			statements(step.Then)
			statements(step.Else)
			for _, branch := range step.ElseIf {
				expression(branch.If)
				statements(branch.Then)
			}
		}
	}
	for _, rule := range config.Rules {
		if rule.Enabled && rule.Program != nil {
			statements(rule.Program.Steps)
		}
	}
	out := []string{}
	for field := range seen {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func retainRawFields(r *matchingRecord, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	root, err := decodeRaw(r.Payload)
	if err != nil {
		return err
	}
	r.RawFields = filters.Env{}
	for _, field := range fields {
		r.RawFields[field] = rawValue(root, field)
	}
	return nil
}

func (s *Store) SourceFields(ctx context.Context, sourceID int64) ([]SourceField, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var count, cached int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_records WHERE source_id=?`, sourceID).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("This source has no records.")
	}
	err = tx.QueryRowContext(ctx, `SELECT record_count FROM source_field_catalog_state WHERE source_id=?`, sourceID).Scan(&cached)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if cached != count {
		found := map[string]SourceField{}
		var walk func(any, string, string)
		walk = func(value any, path, label string) {
			kind := filters.Any
			switch value.(type) {
			case nil:
				kind = filters.Null
			case string:
				kind = filters.Text
			case bool:
				kind = filters.Boolean
			case json.Number:
				kind = filters.Number
			case map[string]any:
				kind = filters.Object
			case []any:
				kind = filters.List
			}
			if filters.ValidRawField("raw:" + path) {
				fieldLabel := label
				if path == "" {
					fieldLabel = "Source record"
				}
				field := SourceField{"raw:" + path, kind, fieldLabel}
				found[field.Path+":"+string(field.Type)] = field
			}
			if object, ok := value.(map[string]any); ok {
				for key, item := range object {
					next := key
					if label != "" {
						next = label + " / " + key
					}
					walk(item, path+"/"+pointerPart(key), next)
				}
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT payload FROM source_records WHERE source_id=? ORDER BY id`, sourceID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			if err = ctx.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			var payload string
			if err = rows.Scan(&payload); err != nil {
				rows.Close()
				return nil, err
			}
			root, err := decodeRaw(payload)
			if err != nil {
				rows.Close()
				return nil, err
			}
			walk(root, "", "")
			if object, ok := root.(map[string]any); ok {
				columns, _ := object["columns"].([]any)
				values, _ := object["values"].([]any)
				for i, column := range columns {
					name, ok := column.(string)
					if !ok || i >= len(values) {
						continue
					}
					v, err := filters.From(values[i])
					if err != nil {
						continue
					}
					field := SourceField{"column:/" + pointerPart(name), v.Type, name + " (CSV)"}
					found[field.Path+":"+string(field.Type)] = field
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM source_field_catalog WHERE source_id=?`, sourceID); err != nil {
			return nil, err
		}
		for _, field := range found {
			if _, err = tx.ExecContext(ctx, `INSERT INTO source_field_catalog(source_id,path,kind,label) VALUES(?,?,?,?)`, sourceID, field.Path, field.Type, field.Label); err != nil {
				return nil, err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_field_catalog_state(source_id,record_count) VALUES(?,?) ON CONFLICT(source_id) DO UPDATE SET record_count=excluded.record_count`, sourceID, count); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT path,kind,label FROM source_field_catalog WHERE source_id=? ORDER BY label,path,kind`, sourceID)
	if err != nil {
		return nil, err
	}
	out := []SourceField{}
	for rows.Next() {
		var field SourceField
		if err = rows.Scan(&field.Path, &field.Type, &field.Label); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, field)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

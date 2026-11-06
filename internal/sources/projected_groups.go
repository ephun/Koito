package sources

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type groupSnapshotItem struct {
	Field, Label string
	Assigned     bool
	RecordID     int64
}

func groupSnapshot(ctx context.Context, tx *sql.Tx) (map[string]groupSnapshotItem, error) {
	result := map[string]groupSnapshotItem{}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.title,COALESCE(g.title,n.display_name,r.title),COALESCE(c.various_artists,r.various_artists),ng.group_id IS NOT NULL,c.album_id IS NOT NULL FROM releases_with_title r LEFT JOIN native_release_groups ng ON ng.release_id=r.id LEFT JOIN album_groups g ON g.id=ng.group_id LEFT JOIN catalog_names n ON n.kind='album' AND n.item_id=r.id LEFT JOIN album_classification c ON c.album_id=r.id ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var title, label string
		var various, grouped, classified bool
		if err = rows.Scan(&id, &title, &label, &various, &grouped, &classified); err != nil {
			rows.Close()
			return nil, err
		}
		result[fmt.Sprint("album:", id)] = groupSnapshotItem{Field: "Album group", Label: label, Assigned: grouped}
		result[fmt.Sprint("classification:", id)] = groupSnapshotItem{Field: "Various Artists: " + title, Label: fmt.Sprint(various), Assigned: classified}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT 'play:'||p.play_key,p.representative_record,COALESCE(g.title,n.display_name,r.title),COALESCE(s.group_id,ng.group_id,0)>0 FROM source_plays p JOIN tracks t ON t.id=p.track_id JOIN releases_with_title r ON r.id=t.release_id LEFT JOIN source_play_groupings s ON s.play_key=p.play_key LEFT JOIN native_release_groups ng ON ng.release_id=r.id LEFT JOIN album_groups g ON g.id=COALESCE(s.group_id,ng.group_id,0) LEFT JOIN catalog_names n ON n.kind='album' AND n.item_id=r.id UNION ALL SELECT 'aggregate:'||p.aggregate_key,p.representative_record,COALESCE(g.title,n.display_name,r.title),COALESCE(s.group_id,ng.group_id,0)>0 FROM source_aggregates p JOIN tracks t ON t.id=p.track_id JOIN releases_with_title r ON r.id=t.release_id LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=p.aggregate_key LEFT JOIN native_release_groups ng ON ng.release_id=r.id LEFT JOIN album_groups g ON g.id=COALESCE(s.group_id,ng.group_id,0) LEFT JOIN catalog_names n ON n.kind='album' AND n.item_id=r.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var item groupSnapshotItem
		item.Field = "Album group"
		if err = rows.Scan(&key, &item.RecordID, &item.Label, &item.Assigned); err != nil {
			rows.Close()
			return nil, err
		}
		result[key] = item
	}
	err = rows.Err()
	rows.Close()
	return result, err
}

func groupDifferences(before, after map[string]groupSnapshotItem) []NamingExample {
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := []string{}
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	result := []NamingExample{}
	seen := map[[3]string]int{}
	groupCount, classCount := 0, 0
	for _, key := range ordered {
		a, old := before[key]
		b, next := after[key]
		if old && next && a.Label == b.Label {
			continue
		}
		if !old {
			if !b.Assigned {
				continue
			}
			a.Label = "Absent"
		}
		if !next {
			if !a.Assigned {
				continue
			}
			b.Label = "Absent"
			b.Field = a.Field
			b.RecordID = a.RecordID
		}
		identity := [3]string{b.Field, a.Label, b.Label}
		if index, ok := seen[identity]; ok {
			if result[index].RecordID == 0 && b.RecordID > 0 {
				result[index].RecordID = b.RecordID
			}
			continue
		}
		if b.Field == "Album group" {
			if groupCount == 12 {
				continue
			}
			groupCount++
		} else {
			if classCount == 12 {
				continue
			}
			classCount++
		}
		seen[identity] = len(result)
		result = append(result, NamingExample{RecordID: b.RecordID, Field: b.Field, Before: a.Label, After: b.Label})
	}
	return result
}

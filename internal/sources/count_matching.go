package sources

import "github.com/gabehf/koito/internal/filters"

// Navidrome increments the cumulative counter when it appends a scrobble row.
// Matching can identify several of those increments as one play. Count's
// starting programs subtract that known overlap from the interpreted total.
// The reported count and every supporting row remain unchanged.
func matchedScrobbleRepeats(track LibraryTrack, library LibraryEvidence, byID map[int64]matchingRecord, current filters.Env, plans ...*matchPlan) filters.Value {
	id, valid := current["count.snapshot"].Lookup("record_id").Int64()
	if !valid {
		return filters.Unknown("Choose a record for names and dates before reading matched scrobble repeats.")
	}
	selected := LibrarySnapshot{}
	for _, snapshot := range track.Snapshots {
		if snapshot.RecordID == id {
			selected = snapshot
			break
		}
	}
	r, exists := byID[id]
	if !exists || r.Service != "navidrome" {
		return filters.Integer(0)
	}
	if len(plans) == 0 || plans[0] == nil {
		return filters.Unknown("Matching is unavailable for these scrobbles.")
	}
	until := libraryDate(selected.LastPlayed)
	if until.IsZero() {
		return filters.Integer(0)
	}
	events := map[string]bool{}
	groups := map[string]bool{}
	for _, other := range plans[0].Records {
		if other.SourceID != r.SourceID || other.FileHash != r.FileHash || other.Parser != r.Parser || other.Values.NativeLibraryID != library.LibraryID || other.Values.NativeTrackID != track.PersistentID || other.Values.EventID == "" || other.Values.Time.After(until) {
			continue
		}
		key := plans[0].Keys[other.ID]
		if key == "" {
			continue
		}
		events[other.Values.EventID] = true
		groups[key] = true
	}
	if selected.Count != nil && int64(len(events)) > *selected.Count {
		return filters.Unknown("The reported counter is smaller than its retained scrobble history.")
	}
	if len(events) < len(groups) {
		return filters.Unknown("Matching does not establish this counter overlap.")
	}
	return filters.Integer(int64(len(events) - len(groups)))
}

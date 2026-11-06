package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gabehf/koito/internal/logger"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

type catalogArtist struct {
	id                    int64
	name, identifier, key string
	active, named         bool
}

func identityPrograms(config string) (IdentityPrograms, error) {
	var settings struct {
		Rules []IdentityFilter `json:"rules"`
	}
	if err := json.Unmarshal([]byte(config), &settings); err != nil {
		return IdentityPrograms{}, err
	}
	return CompileIdentityPrograms(settings.Rules)
}

// A missing identifier can join one known identity, but cannot bridge two
// conflicting known identities. Ambiguous unknowns retain their original ID.
func compatibleIdentity(id int64, identifier string, candidates []catalogArtist) int64 {
	known := map[string]bool{}
	for _, c := range candidates {
		if c.identifier != "" {
			known[c.identifier] = true
		}
	}
	best := id
	found := false
	self := false
	for _, c := range candidates {
		self = self || c.id == id
	}
	bestActive, bestNamed := false, false
	for _, c := range candidates {
		compatible := c.identifier == identifier
		if len(known) <= 1 {
			compatible = identifier == "" || c.identifier == "" || c.identifier == identifier
		}
		if len(known) > 1 && identifier == "" {
			compatible = c.id == id
		}
		if !self && !c.active && !c.named {
			continue
		}
		if compatible && (!found || c.named && !bestNamed || c.named == bestNamed && c.active && !bestActive || c.named == bestNamed && c.active == bestActive && c.id < best) {
			best = c.id
			found = true
			bestActive, bestNamed = c.active, c.named
		}
	}
	return best
}

func comparisonEnvironment(kind string, id int64, artist, album, title, identifier string) filters.Env {
	return filters.Env{"catalog.kind": filters.String(kind), "catalog.id": filters.Integer(id),
		"source.service": filters.String("koito"), "original.artist": filters.String(artist),
		"original.album": filters.String(album), "original.title": filters.String(title), "original.identifier": filters.String(identifier)}
}

func loadCatalogArtists(ctx context.Context, db Database, programs IdentityPrograms) ([]catalogArtist, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.name,COALESCE(a.musicbrainz_id,''),EXISTS(SELECT 1 FROM artist_tracks ar WHERE ar.artist_id=a.id AND (EXISTS(SELECT 1 FROM listens l WHERE l.track_id=ar.track_id) OR EXISTS(SELECT 1 FROM source_aggregates g WHERE g.track_id=ar.track_id AND g.undated_count>0))),EXISTS(SELECT 1 FROM catalog_names n WHERE n.kind='artist' AND n.item_id=a.id) FROM artists_with_name a ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	items := []catalogArtist{}
	for rows.Next() {
		var a catalogArtist
		if err = rows.Scan(&a.id, &a.name, &a.identifier, &a.active, &a.named); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Source-scoped comparisons describe contributions. Only unanimous evidence
	// supplies a whole-artist comparison. Split evidence is resolved per track.
	rows, err = db.QueryContext(ctx, `SELECT a.artist_id,c.artist FROM track_catalog_comparisons c JOIN artist_tracks a ON a.track_id=c.track_id
 WHERE (SELECT COUNT(*) FROM artist_tracks b WHERE b.track_id=c.track_id)=1 GROUP BY a.artist_id,c.artist`)
	if err != nil {
		return nil, err
	}
	keys := map[int64][]string{}
	for rows.Next() {
		var id int64
		var key string
		if err = rows.Scan(&id, &key); err != nil {
			rows.Close()
			return nil, err
		}
		keys[id] = append(keys[id], key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	nativeRows, err := db.QueryContext(ctx, `SELECT a.artist_id,t.id,t.title,COALESCE(r.title,'') FROM artist_tracks a JOIN tracks_with_title t ON t.id=a.track_id LEFT JOIN releases_with_title r ON r.id=t.release_id WHERE (SELECT COUNT(*) FROM artist_tracks b WHERE b.track_id=t.id)=1 AND EXISTS(SELECT 1 FROM listens l WHERE l.track_id=t.id AND l.client NOT IN('sources:spotify-v1','sources:filters-v1')) GROUP BY a.artist_id,t.id`)
	if err != nil {
		return nil, err
	}
	type nativeArtist struct {
		id, track    int64
		title, album string
	}
	native := []nativeArtist{}
	for nativeRows.Next() {
		var n nativeArtist
		if err = nativeRows.Scan(&n.id, &n.track, &n.title, &n.album); err != nil {
			nativeRows.Close()
			return nil, err
		}
		native = append(native, n)
	}
	err = nativeRows.Err()
	nativeRows.Close()
	if err != nil {
		return nil, err
	}
	byID := map[int64]catalogArtist{}
	for _, a := range items {
		byID[a.id] = a
	}
	for _, n := range native {
		a := byID[n.id]
		album := n.album
		if album == "Unknown Album" {
			album = ""
		}
		v, _, err := programs.Resolve(ctx, comparisonEnvironment("track", n.track, a.name, album, n.title, ""), MatchIdentity{Artist: a.name, Album: album, Title: n.title})
		if err != nil {
			return nil, err
		}
		found := false
		for _, key := range keys[n.id] {
			found = found || key == v.Artist
		}
		if !found {
			keys[n.id] = append(keys[n.id], v.Artist)
		}
	}
	for i := range items {
		a := &items[i]
		value, _, err := programs.Resolve(ctx, comparisonEnvironment("artist", a.id, a.name, "", "", a.identifier), MatchIdentity{Artist: a.name})
		if err != nil {
			return nil, err
		}
		a.key = value.Artist
		if len(keys[a.id]) == 1 {
			a.key = keys[a.id][0]
		}
		value, _, err = programs.Catalog().Resolve(ctx, comparisonEnvironment("artist", a.id, a.name, "", "", a.identifier), MatchIdentity{Artist: a.key})
		if err != nil {
			return nil, err
		}
		a.key = value.Artist
		// Empty names do not establish an identity.
		if a.key == "" {
			a.key = "artist:" + strconv.FormatInt(a.id, 10)
		}
	}
	return items, nil
}

// RefreshArtistMatches publishes comparison links without changing aliases,
// catalog credits, identifiers, or source evidence.
func RefreshArtistMatches(ctx context.Context, db Database, config string) error {
	programs, err := identityPrograms(config)
	if err != nil {
		return err
	}
	if !programs.Active() {
		_, err = db.ExecContext(ctx, `DELETE FROM music_artist_matches`)
		return err
	}
	artists, err := loadCatalogArtists(ctx, db, programs)
	if err != nil {
		return err
	}
	families := map[string][]catalogArtist{}
	for _, a := range artists {
		families[a.key] = append(families[a.key], a)
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM music_artist_matches`); err != nil {
		return err
	}
	for _, a := range artists {
		target := compatibleIdentity(a.id, a.identifier, families[a.key])
		if _, err = db.ExecContext(ctx, `INSERT INTO music_artist_matches(artist_id,matched_artist_id,comparison_key) VALUES(?,?,?)`, a.id, target, a.key); err != nil {
			return err
		}
	}
	return nil
}

type catalogAppearance struct {
	id, release                                    int64
	title, album, identifier, artistNames, credits string
	comparisonArtist, comparisonAlbum              string
	sourceComparison                               bool
}

// RefreshCatalogMatches derives artist contributions and album report groups
// from the same comparison values used by music matching. Grouping filters
// retain precedence, including explicit source exceptions.
func RefreshCatalogMatches(ctx context.Context, db Database, config string) error {
	return refreshCatalogMatches(ctx, db, config, "", 0)
}

// Incoming tracks refresh their comparison families. Catalog-wide changes
// rebuild every family in the same transaction.
func AssignCatalogMatches(ctx context.Context, db Database, config, kind string, id int64) error {
	if kind == "artist" {
		return RefreshCatalogMatches(ctx, db, config)
	}
	return refreshCatalogMatches(ctx, db, config, kind, id)
}

func refreshCatalogMatches(ctx context.Context, db Database, config, kind string, id int64) error {
	started := time.Now()
	stage := func(name string, count int) {
		logger.FromContext(ctx).Debug().Str("stage", name).Int("items", count).Dur("elapsed", time.Since(started)).Msg("Matching music catalog")
	}

	full := kind == ""
	if full {
		if err := RefreshArtistMatches(ctx, db, config); err != nil {
			return err
		}
	}
	programs, err := identityPrograms(config)
	if err != nil {
		return err
	}
	if !programs.Active() {
		for _, q := range []string{`DELETE FROM music_track_artists`, `DELETE FROM music_release_groups`, `DELETE FROM music_album_comparisons`, `DELETE FROM album_group_members WHERE group_id IN(SELECT id FROM album_groups WHERE json_extract(group_key,'$[0]')='music')`} {
			if _, err = db.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		return nil
	}
	query := `SELECT t.id,t.release_id,t.title,r.title,COALESCE(r.musicbrainz_id,''),
 c.artist,c.album FROM tracks_with_title t JOIN releases_with_title r ON r.id=t.release_id LEFT JOIN track_catalog_comparisons c ON c.track_id=t.id`
	args := []any{}
	if kind == "track" {
		query += ` WHERE t.id=?`
		args = append(args, id)
	}
	if kind == "album" {
		query += ` WHERE t.release_id=?`
		args = append(args, id)
	}
	query += ` ORDER BY t.id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	items := []catalogAppearance{}
	for rows.Next() {
		var a catalogAppearance
		var artist, album *string
		if err = rows.Scan(&a.id, &a.release, &a.title, &a.album, &a.identifier, &artist, &album); err != nil {
			rows.Close()
			return err
		}
		if artist != nil {
			a.sourceComparison = true
			a.comparisonArtist = *artist
			a.comparisonAlbum = *album
		}
		items = append(items, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	stage("Artists", len(items))
	rows, err = db.QueryContext(ctx, `SELECT a.id,a.name,COALESCE(a.musicbrainz_id,''),COALESCE(m.comparison_key,a.name),EXISTS(SELECT 1 FROM artist_tracks ar WHERE ar.artist_id=a.id AND (EXISTS(SELECT 1 FROM listens l WHERE l.track_id=ar.track_id) OR EXISTS(SELECT 1 FROM source_aggregates g WHERE g.track_id=ar.track_id AND g.undated_count>0))),EXISTS(SELECT 1 FROM catalog_names n WHERE n.kind='artist' AND n.item_id=a.id) FROM artists_with_name a LEFT JOIN music_artist_matches m ON m.artist_id=a.id ORDER BY a.id`)
	if err != nil {
		return err
	}
	artists := []catalogArtist{}
	for rows.Next() {
		var a catalogArtist
		if err = rows.Scan(&a.id, &a.name, &a.identifier, &a.key, &a.active, &a.named); err != nil {
			rows.Close()
			return err
		}
		artists = append(artists, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Read raw credit ordering once. Correlating the artists view from every
	// appearance can cause a full catalog scan for each newly imported track.
	creditQuery := `SELECT track_id,artist_id FROM artist_tracks`
	creditArgs := []any{}
	if kind == "track" {
		creditQuery += ` WHERE track_id=?`
		creditArgs = append(creditArgs, id)
	}
	if kind == "album" {
		creditQuery += ` WHERE track_id IN(SELECT id FROM tracks WHERE release_id=?)`
		creditArgs = append(creditArgs, id)
	}
	creditQuery += ` ORDER BY track_id,artist_id`
	creditRows, err := db.QueryContext(ctx, creditQuery, creditArgs...)
	if err != nil {
		return err
	}
	trackCredits := map[int64][]int64{}
	for creditRows.Next() {
		var track, artist int64
		if err = creditRows.Scan(&track, &artist); err != nil {
			creditRows.Close()
			return err
		}
		trackCredits[track] = append(trackCredits[track], artist)
	}
	err = creditRows.Err()
	creditRows.Close()
	if err != nil {
		return err
	}
	artistNames := map[int64][]string{}
	for _, a := range artists {
		artistNames[a.id] = append(artistNames[a.id], a.name)
	}
	for i := range items {
		names, ids := []string{}, []string{}
		for _, artist := range trackCredits[items[i].id] {
			ids = append(ids, strconv.FormatInt(artist, 10))
			names = append(names, artistNames[artist]...)
		}
		raw, _ := json.Marshal(names)
		items[i].artistNames = string(raw)
		items[i].credits = strings.Join(ids, ",")
	}
	families := map[string][]catalogArtist{}
	byID := map[int64]catalogArtist{}
	for _, a := range artists {
		families[a.key] = append(families[a.key], a)
		byID[a.id] = a
	}
	stage("Release flags", len(artists))
	if full {
		for _, query := range []string{`DELETE FROM music_track_artists`, `DELETE FROM music_release_groups`, `DELETE FROM music_album_comparisons`,
			`DELETE FROM album_group_members WHERE group_id IN(SELECT id FROM album_groups WHERE json_extract(group_key,'$[0]')='music')`} {
			if _, err = db.ExecContext(ctx, query); err != nil {
				return err
			}
		}
	}
	type releaseContribution struct {
		track, release  int64
		key, identifier string
	}
	contributions := []releaseContribution{}
	releaseFlags := map[int64]catalogArtist{}
	rows, err = db.QueryContext(ctx, `SELECT r.id,EXISTS(SELECT 1 FROM tracks t WHERE t.release_id=r.id AND (EXISTS(SELECT 1 FROM listens l WHERE l.track_id=t.id) OR EXISTS(SELECT 1 FROM source_aggregates a WHERE a.track_id=t.id AND a.undated_count>0))),EXISTS(SELECT 1 FROM catalog_names n WHERE n.kind='album' AND n.item_id=r.id) FROM releases r`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a catalogArtist
		if err = rows.Scan(&a.id, &a.active, &a.named); err != nil {
			rows.Close()
			return err
		}
		releaseFlags[a.id] = a
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	stage("Credits", len(items))
	affected := map[string]bool{}
	// Read structured release credits once. A predicate above the grouped view
	// can otherwise repeat its complete scan for every track appearance.
	displayedCredits := map[int64][]string{}
	rawReleaseCredits := map[int64][]string{}
	correctedReleases := map[int64]bool{}
	for _, input := range []struct {
		query  string
		values map[int64][]string
	}{
		{`SELECT release_id,artist_id FROM displayed_artist_releases ORDER BY release_id,artist_id`, displayedCredits},
		{`SELECT release_id,artist_id FROM artist_releases ORDER BY release_id,artist_id`, rawReleaseCredits},
	} {
		rows, e := db.QueryContext(ctx, input.query)
		if e != nil {
			return e
		}
		for rows.Next() {
			var release, artist int64
			if e = rows.Scan(&release, &artist); e != nil {
				rows.Close()
				return e
			}
			input.values[release] = append(input.values[release], strconv.FormatInt(artist, 10))
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	rows, err = db.QueryContext(ctx, `SELECT item_id FROM catalog_credit_sets WHERE kind='album'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var release int64
		if err = rows.Scan(&release); err != nil {
			rows.Close()
			return err
		}
		correctedReleases[release] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	grouping, err := CompileGroupingProgramsFromConfig(config)
	if err != nil {
		return err
	}
	groupDisplays := map[int64]filters.Env{}
	various := map[int64]bool{}
	if grouping.Active() {
		groupDisplays, err = AlbumDisplays(ctx, db, 0)
		if err != nil {
			return err
		}
		rows, err = db.QueryContext(ctx, `SELECT id,various_artists FROM releases`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var value bool
			if err = rows.Scan(&id, &value); err != nil {
				rows.Close()
				return err
			}
			various[id] = value
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	separate := map[int64]bool{}
	checked := map[int64]bool{}
	catalogPrograms := programs.Catalog()
	for index, a := range items {
		if index%10000 == 0 {
			stage("Comparisons", index)
		}
		if !full {
			var oldKey string
			if e := db.QueryRowContext(ctx, `SELECT comparison_key FROM music_album_comparisons WHERE track_id=?`, a.id).Scan(&oldKey); e == nil {
				affected[oldKey] = true
			} else if e != sql.ErrNoRows {
				return e
			}
			if _, err = db.ExecContext(ctx, `DELETE FROM music_track_artists WHERE track_id=?`, a.id); err != nil {
				return err
			}
			if _, err = db.ExecContext(ctx, `DELETE FROM music_release_groups WHERE track_id=?`, a.id); err != nil {
				return err
			}
		}
		var names []string
		if err = json.Unmarshal([]byte(a.artistNames), &names); err != nil {
			return err
		}
		artist := strings.Join(names, " & ")
		album := a.album
		if album == "Unknown Album" {
			album = ""
		}
		origin, err := TrackOrigin(ctx, db, a.id)
		if err != nil {
			return err
		}
		value, _, err := programs.Resolve(ctx, comparisonEnvironment("track", origin, artist, album, a.title, ""), MatchIdentity{Artist: artist, Album: album, Title: a.title})
		if err != nil {
			return err
		}
		if a.sourceComparison {
			value.Artist = a.comparisonArtist
			value.Album = a.comparisonAlbum
		}
		if len(names) == 1 {
			id, _ := strconv.ParseInt(a.credits, 10, 64)
			catalogValue, _, err := catalogPrograms.Resolve(ctx, comparisonEnvironment("artist", id, artist, "", "", byID[id].identifier), MatchIdentity{Artist: value.Artist})
			if err != nil {
				return err
			}
			value.Artist = catalogValue.Artist
		}
		if len(names) == 1 {
			id, err := strconv.ParseInt(a.credits, 10, 64)
			if err != nil {
				return err
			}
			// An explicit artist-credit correction supersedes the reported credit.
			var corrected bool
			if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_credit_sets WHERE kind='track' AND item_id=?)`, a.id).Scan(&corrected); err != nil {
				return err
			}
			if !corrected {
				target := compatibleIdentity(id, byID[id].identifier, families[value.Artist])
				if _, err = db.ExecContext(ctx, `INSERT INTO music_track_artists(track_id,original_artist_id,artist_id) VALUES(?,?,?)`, a.id, id, target); err != nil {
					return err
				}
			}
		}
		// Release comparisons preserve structured album credits. Guest track
		// credits do not redefine the album artist.
		credits := strings.Join(displayedCredits[a.release], ",")
		rawCredits := strings.Join(rawReleaseCredits[a.release], ",")
		if rawCredits == a.credits && len(names) == 1 {
			if !correctedReleases[a.release] {
				id, _ := strconv.ParseInt(a.credits, 10, 64)
				credits = strconv.FormatInt(compatibleIdentity(id, byID[id].identifier, families[value.Artist]), 10)
			}
		}
		albumEnv := comparisonEnvironment("album", a.release, "", album, "", a.identifier)
		if !strings.Contains(rawCredits, ",") && rawCredits != "" {
			id, _ := strconv.ParseInt(rawCredits, 10, 64)
			albumEnv["original.artist"] = filters.String(byID[id].name)
		} else {
			delete(albumEnv, "original.artist")
		}
		catalogValue, _, err := catalogPrograms.Resolve(ctx, albumEnv, MatchIdentity{Album: value.Album})
		if err != nil {
			return err
		}
		value.Album = catalogValue.Album
		key, _ := json.Marshal([]string{credits, value.Album})
		if !checked[a.release] && grouping.Active() {
			ids, names := []filters.Value{}, []filters.Value{}
			for _, rawID := range rawReleaseCredits[a.release] {
				id, e := strconv.ParseInt(rawID, 10, 64)
				if e != nil {
					return e
				}
				ids = append(ids, filters.Integer(id))
				names = append(names, filters.String(byID[id].name))
			}
			env := AlbumGroupingEnvironment(a.release, a.album, various[a.release], filters.Env{"catalog.artist_ids": filters.Values(ids...), "original.artists": filters.Values(names...)})
			env = WithAlbumDisplay(env, groupDisplays[a.release])
			label, assigned, _, e := grouping.Resolve(ctx, env)
			if e != nil {
				return e
			}
			separate[a.release] = assigned && label == ""
			checked[a.release] = true
		}
		if separate[a.release] {
			key, _ = json.Marshal([]string{"separate", strconv.FormatInt(a.release, 10)})
		}
		contributions = append(contributions, releaseContribution{a.id, a.release, string(key), a.identifier})
		if _, err = db.ExecContext(ctx, `INSERT INTO music_album_comparisons(track_id,release_id,comparison_key,artist,artist_credits,identifier) VALUES(?,?,?,?,?,?) ON CONFLICT(track_id) DO UPDATE SET release_id=excluded.release_id,comparison_key=excluded.comparison_key,artist=excluded.artist,artist_credits=excluded.artist_credits,identifier=excluded.identifier`, a.id, a.release, string(key), value.Artist, credits, a.identifier); err != nil {
			return err
		}
		affected[string(key)] = true
	}
	buckets := map[string][]releaseContribution{}
	stage("Groups", len(contributions))
	for _, c := range contributions {
		buckets[c.key] = append(buckets[c.key], c)
	}
	if !full {
		buckets = map[string][]releaseContribution{}
		for key := range affected {
			for _, q := range []string{`DELETE FROM music_release_groups WHERE group_id IN(SELECT id FROM album_groups WHERE json_extract(group_key,'$[0]')='music' AND json_extract(group_key,'$[1]')=?)`, `DELETE FROM album_group_members WHERE group_id IN(SELECT id FROM album_groups WHERE json_extract(group_key,'$[0]')='music' AND json_extract(group_key,'$[1]')=?)`} {
				if _, err = db.ExecContext(ctx, q, key); err != nil {
					return err
				}
			}
			rows, err = db.QueryContext(ctx, `SELECT track_id,release_id,identifier FROM music_album_comparisons WHERE comparison_key=? ORDER BY track_id`, key)
			if err != nil {
				return err
			}
			for rows.Next() {
				var c releaseContribution
				c.key = key
				if err = rows.Scan(&c.track, &c.release, &c.identifier); err != nil {
					rows.Close()
					return err
				}
				buckets[key] = append(buckets[key], c)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
	}
	keys := []string{}
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := buckets[key]
		candidates := []catalogArtist{}
		seen := map[int64]bool{}
		for _, c := range bucket {
			if !seen[c.release] {
				candidate := releaseFlags[c.release]
				candidate.identifier = c.identifier
				candidates = append(candidates, candidate)
				seen[c.release] = true
			}
		}
		partitions := map[int64][]releaseContribution{}
		for _, c := range bucket {
			canonical := compatibleIdentity(c.release, c.identifier, candidates)
			partitions[canonical] = append(partitions[canonical], c)
		}
		canonicalIDs := []int64{}
		for id := range partitions {
			canonicalIDs = append(canonicalIDs, id)
		}
		sort.Slice(canonicalIDs, func(i, j int) bool { return canonicalIDs[i] < canonicalIDs[j] })
		for _, canonical := range canonicalIDs {
			members := partitions[canonical]
			distinct := map[int64]bool{}
			for _, c := range members {
				distinct[c.release] = true
			}
			if len(distinct) < 2 {
				continue
			}
			var title string
			if err = db.QueryRowContext(ctx, `SELECT COALESCE((SELECT display_name FROM catalog_names WHERE kind='album' AND item_id=r.id),r.title) FROM releases_with_title r WHERE r.id=?`, canonical).Scan(&title); err != nil {
				return err
			}
			groupKey, _ := json.Marshal([]string{"music", key, strconv.FormatInt(canonical, 10)})
			if _, err = db.ExecContext(ctx, `INSERT INTO album_groups(group_key,title,canonical_release_id) VALUES(?,?,?) ON CONFLICT(group_key) DO UPDATE SET title=excluded.title,canonical_release_id=excluded.canonical_release_id`, string(groupKey), title, canonical); err != nil {
				return err
			}
			var group int64
			if err = db.QueryRowContext(ctx, `SELECT id FROM album_groups WHERE group_key=?`, string(groupKey)).Scan(&group); err != nil {
				return err
			}
			for release := range distinct {
				if _, err = db.ExecContext(ctx, `INSERT INTO album_group_members(group_id,release_id) VALUES(?,?) ON CONFLICT DO NOTHING`, group, release); err != nil {
					return err
				}
			}
			for _, c := range members {
				if _, err = db.ExecContext(ctx, `INSERT INTO music_release_groups(track_id,group_id) VALUES(?,?)`, c.track, group); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

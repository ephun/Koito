package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gabehf/koito/internal/catalog"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/models"
	"github.com/gabehf/koito/internal/music"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
)

func (s *Sqlite) SaveListen(ctx context.Context, opts db.SaveListenOpts) error {
	if opts.TrackID == 0 {
		return errors.New("SaveListen: required parameter TrackID missing")
	}
	if opts.Time.IsZero() {
		opts.Time = time.Now()
	}
	client := ""
	if opts.Client != "" {
		client = opts.Client
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	native, created, err := music.NativeAppearance(ctx, tx, int64(opts.TrackID))
	if err != nil {
		return err
	}
	opts.TrackID = int32(native)
	if created {
		if err = refreshNativeCatalogItem(ctx, tx, "track", native); err != nil {
			return err
		}
	}
	var first bool
	if err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM listens WHERE track_id=?)`, native).Scan(&first); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO listens (track_id, listened_at, user_id, client) VALUES (?,?,?,?)`,
		opts.TrackID, opts.Time.Unix(), opts.UserID, client,
	)
	if err != nil {
		return err
	}
	if first {
		var config string
		if err = tx.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&config); err != nil {
			return err
		}
		if err = music.AssignCatalogMatches(ctx, tx, config, "track", native); err != nil {
			return err
		}
	}
	var split bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM music_album_comparisons c JOIN artist_tracks a ON a.track_id=c.track_id JOIN music_artist_matches m ON m.artist_id=a.artist_id WHERE c.track_id=? AND (SELECT COUNT(*) FROM artist_tracks b WHERE b.track_id=c.track_id)=1 AND (c.artist<>m.comparison_key OR (m.matched_artist_id<>a.artist_id AND NOT EXISTS(SELECT 1 FROM catalog_names n WHERE n.kind='artist' AND n.item_id=m.matched_artist_id) AND NOT EXISTS(SELECT 1 FROM artist_tracks x JOIN listens l ON l.track_id=x.track_id WHERE x.artist_id=m.matched_artist_id) AND NOT EXISTS(SELECT 1 FROM artist_tracks x JOIN source_aggregates g ON g.track_id=x.track_id WHERE x.artist_id=m.matched_artist_id AND g.undated_count>0))))`, native).Scan(&split); err != nil {
		return err
	}
	if split {
		var config string
		if err = tx.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&config); err != nil {
			return err
		}
		if err = music.RefreshCatalogMatches(ctx, tx, config); err != nil {
			return err
		}
		if err = sources.RefreshMatchedCatalog(ctx, tx); err != nil {
			return err
		}
		sensitive, err := music.CaseSensitive(ctx, tx)
		if err != nil {
			return err
		}
		if err = music.RefreshHistories(ctx, tx, sensitive, config); err != nil {
			return err
		}
		if err = music.RefreshScope(ctx, tx, 0, config); err != nil {
			return err
		}
	}
	if err = music.RefreshHistoryCanonicalForTrack(ctx, tx, native); err != nil {
		return err
	}
	if err = music.RefreshListenScope(ctx, tx, int64(opts.TrackID), opts.Time.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Sqlite) DeleteListen(ctx context.Context, trackId int32, listenedAt time.Time) error {
	if trackId == 0 {
		return errors.New("DeleteListen: required parameter 'trackId' missing")
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM listens WHERE track_id = ? AND listened_at = ? AND play_key=''`,
		trackId, listenedAt.Unix(),
	)
	return err
}

// listenRow is an intermediate scan target used to decouple the main rows
// query from the per-row artistsForTrack sub-query. With MaxOpenConns(1),
// both the count query and the artistsForTrack call would deadlock if
// executed while the outer *sql.Rows is still holding the only connection.
type listenRow struct {
	tagsJSON         string
	albumID          int32
	album            string
	evidence         string
	attributionBasis string
	playKey          string
	recordID         int64
	listenedAt       int64
	trackID          int32
	title            string
}

func (s *Sqlite) GetListensPaginated(ctx context.Context, opts db.GetItemsOpts) (*db.PaginatedResponse[*models.Listen], error) {
	if opts.Limit == 0 {
		opts.Limit = defaultItemsPerPage
	}
	offset := (opts.Page - 1) * opts.Limit
	t1, t2 := db.TimeframeToTimeRange(opts.Timeframe)

	// Count queries run first, before any main rows query is opened, so
	// they never compete with an open *sql.Rows for the single connection.
	var count int64
	switch {
	case opts.AlbumGroupID > 0:
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM album_report_events WHERE listened_at BETWEEN ? AND ? AND album_group_id=?`, t1.Unix(), t2.Unix(), opts.AlbumGroupID).Scan(&count)
		if err != nil {
			return nil, err
		}
	case opts.TrackID > 0:
		s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM displayed_listens WHERE listened_at BETWEEN ? AND ? AND `+historyPredicate("track_id"),
			t1.Unix(), t2.Unix(), opts.TrackID).Scan(&count)
	case opts.AlbumID > 0:
		s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM displayed_listens l JOIN tracks t ON l.track_id = t.id
			WHERE l.listened_at BETWEEN ? AND ? AND t.release_id = ?`,
			t1.Unix(), t2.Unix(), opts.AlbumID).Scan(&count)
	case opts.ArtistID > 0:
		s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM displayed_listens l JOIN displayed_artist_tracks at2 ON l.track_id = at2.track_id
			WHERE l.listened_at BETWEEN ? AND ? AND at2.artist_id = ?`,
			t1.Unix(), t2.Unix(), opts.ArtistID).Scan(&count)
	default:
		s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM displayed_listens WHERE listened_at BETWEEN ? AND ?`,
			t1.Unix(), t2.Unix()).Scan(&count)
	}

	// Open and fully drain the main rows query, then close before any
	// sub-queries so the connection is free for artistsForTrack.
	var rows *sql.Rows
	var err error
	switch {
	case opts.TrackID > 0:
		rows, err = s.db.QueryContext(ctx, `
			SELECT l.listened_at, l.track_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='track' AND cn.item_id=t.id),t.title), t.release_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=t.release_id),COALESCE((SELECT alias FROM release_aliases WHERE release_id=t.release_id AND is_primary=1),'')), l.play_key, COALESCE((SELECT representative_record FROM source_plays p WHERE p.policy='filters-v1' AND p.play_key=l.play_key),0),COALESCE((SELECT evidence_level FROM source_play_details d WHERE d.play_key=l.play_key),'observed'),COALESCE((SELECT json_extract(attribution_json,'$.basis') FROM source_play_details d WHERE d.play_key=l.play_key),''),COALESCE((SELECT tags_json FROM source_play_tags tags WHERE tags.play_key=l.play_key),'[]')
			FROM displayed_listens l
			JOIN tracks_with_title t ON l.track_id = t.id
			WHERE l.listened_at BETWEEN ? AND ? AND `+historyPredicate("t.id")+`
			ORDER BY l.listened_at DESC,l.track_id,l.play_key LIMIT ? OFFSET ?`,
			t1.Unix(), t2.Unix(), opts.TrackID, opts.Limit, offset,
		)
	case opts.AlbumGroupID > 0:
		rows, err = s.db.QueryContext(ctx, `
			SELECT l.listened_at, l.track_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='track' AND cn.item_id=t.id),t.title), t.release_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=t.release_id),COALESCE((SELECT alias FROM release_aliases WHERE release_id=t.release_id AND is_primary=1),'')), l.play_key, COALESCE((SELECT representative_record FROM source_plays p WHERE p.policy='filters-v1' AND p.play_key=l.play_key),0),COALESCE((SELECT evidence_level FROM source_play_details d WHERE d.play_key=l.play_key),'observed'),COALESCE((SELECT json_extract(attribution_json,'$.basis') FROM source_play_details d WHERE d.play_key=l.play_key),''),COALESCE((SELECT tags_json FROM source_play_tags tags WHERE tags.play_key=l.play_key),'[]')
			FROM album_report_events l
			JOIN tracks_with_title t ON l.track_id = t.id
			WHERE l.listened_at BETWEEN ? AND ? AND l.album_group_id = ?
			ORDER BY l.listened_at DESC,l.track_id,l.play_key LIMIT ? OFFSET ?`,
			t1.Unix(), t2.Unix(), opts.AlbumGroupID, opts.Limit, offset,
		)
	case opts.AlbumID > 0:
		rows, err = s.db.QueryContext(ctx, `
			SELECT l.listened_at, l.track_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='track' AND cn.item_id=t.id),t.title), t.release_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=t.release_id),COALESCE((SELECT alias FROM release_aliases WHERE release_id=t.release_id AND is_primary=1),'')), l.play_key, COALESCE((SELECT representative_record FROM source_plays p WHERE p.policy='filters-v1' AND p.play_key=l.play_key),0),COALESCE((SELECT evidence_level FROM source_play_details d WHERE d.play_key=l.play_key),'observed'),COALESCE((SELECT json_extract(attribution_json,'$.basis') FROM source_play_details d WHERE d.play_key=l.play_key),''),COALESCE((SELECT tags_json FROM source_play_tags tags WHERE tags.play_key=l.play_key),'[]')
			FROM displayed_listens l
			JOIN tracks_with_title t ON l.track_id = t.id
			WHERE l.listened_at BETWEEN ? AND ? AND t.release_id = ?
			ORDER BY l.listened_at DESC,l.track_id,l.play_key LIMIT ? OFFSET ?`,
			t1.Unix(), t2.Unix(), opts.AlbumID, opts.Limit, offset,
		)
	case opts.ArtistID > 0:
		rows, err = s.db.QueryContext(ctx, `
			SELECT l.listened_at, l.track_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='track' AND cn.item_id=t.id),t.title), t.release_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=t.release_id),COALESCE((SELECT alias FROM release_aliases WHERE release_id=t.release_id AND is_primary=1),'')), l.play_key, COALESCE((SELECT representative_record FROM source_plays p WHERE p.policy='filters-v1' AND p.play_key=l.play_key),0),COALESCE((SELECT evidence_level FROM source_play_details d WHERE d.play_key=l.play_key),'observed'),COALESCE((SELECT json_extract(attribution_json,'$.basis') FROM source_play_details d WHERE d.play_key=l.play_key),''),COALESCE((SELECT tags_json FROM source_play_tags tags WHERE tags.play_key=l.play_key),'[]')
			FROM displayed_listens l
			JOIN tracks_with_title t ON l.track_id = t.id
			JOIN displayed_artist_tracks at2 ON t.id = at2.track_id
			WHERE l.listened_at BETWEEN ? AND ? AND at2.artist_id = ?
			ORDER BY l.listened_at DESC,l.track_id,l.play_key LIMIT ? OFFSET ?`,
			t1.Unix(), t2.Unix(), opts.ArtistID, opts.Limit, offset,
		)
	default:
		rows, err = s.db.QueryContext(ctx, `
			SELECT l.listened_at, l.track_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='track' AND cn.item_id=t.id),t.title), t.release_id, COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=t.release_id),COALESCE((SELECT alias FROM release_aliases WHERE release_id=t.release_id AND is_primary=1),'')), l.play_key, COALESCE((SELECT representative_record FROM source_plays p WHERE p.policy='filters-v1' AND p.play_key=l.play_key),0),COALESCE((SELECT evidence_level FROM source_play_details d WHERE d.play_key=l.play_key),'observed'),COALESCE((SELECT json_extract(attribution_json,'$.basis') FROM source_play_details d WHERE d.play_key=l.play_key),''),COALESCE((SELECT tags_json FROM source_play_tags tags WHERE tags.play_key=l.play_key),'[]')
			FROM displayed_listens l
			JOIN tracks_with_title t ON l.track_id = t.id
			WHERE l.listened_at BETWEEN ? AND ?
			ORDER BY l.listened_at DESC,l.track_id,l.play_key LIMIT ? OFFSET ?`,
			t1.Unix(), t2.Unix(), opts.Limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("GetListensPaginated: %w", err)
	}

	var raw []listenRow
	for rows.Next() {
		var r listenRow
		if err := rows.Scan(&r.listenedAt, &r.trackID, &r.title, &r.albumID, &r.album, &r.playKey, &r.recordID, &r.evidence, &r.attributionBasis, &r.tagsJSON); err != nil {
			rows.Close()
			return nil, err
		}
		raw = append(raw, r)
	}
	// Explicitly close before calling artistsForTrack so the connection is free.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	listens := make([]*models.Listen, 0, len(raw))
	for _, r := range raw {
		l := &models.Listen{
			AlbumID:          r.albumID,
			Album:            r.album,
			PlayKey:          r.playKey,
			Evidence:         r.evidence,
			AttributionBasis: r.attributionBasis,
			RecordID:         r.recordID,
			Time:             time.Unix(r.listenedAt, 0).UTC(),
			Track: models.SimpleTrack{
				ID:    r.trackID,
				Title: r.title,
			},
		}
		if err = json.Unmarshal([]byte(r.tagsJSON), &l.Tags); err != nil {
			return nil, err
		}
		l.Track.Artists, err = s.artistsForTrack(ctx, r.trackID)
		if err != nil {
			return nil, err
		}
		imgid, err := s.imageForTrack(ctx, l.Track.ID)
		if err != nil {
			return nil, err
		}
		l.Track.Image = catalog.BuildImageList(imgid)
		listens = append(listens, l)
	}

	return &db.PaginatedResponse[*models.Listen]{
		Items:        listens,
		TotalCount:   count,
		ItemsPerPage: int32(opts.Limit),
		HasNextPage:  int64(offset+len(listens)) < count,
		CurrentPage:  int32(opts.Page),
	}, nil
}

func (s *Sqlite) imageForTrack(ctx context.Context, trackId int32) (*uuid.UUID, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT r.image
		FROM releases r
		JOIN tracks t ON r.id = t.release_id
		WHERE t.id = ? AND r.image IS NOT NULL`,
		trackId,
	)
	var imageid *uuid.UUID
	err := row.Scan(&imageid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("imageForTrack: %w", err)
	}
	return imageid, nil
}

func (s *Sqlite) GetFirstListenUnix(ctx context.Context) (int64, error) {
	row := s.db.QueryRow(`
		SELECT listened_at FROM displayed_listens ORDER BY listened_at ASC LIMIT 1;`)
	var unix int64
	err := row.Scan(&unix)
	if err != nil {
		return 0, fmt.Errorf("GetFirstListenUnix: %w", err)
	}

	return unix, nil
}

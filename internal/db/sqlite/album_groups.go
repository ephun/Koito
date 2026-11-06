package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gabehf/koito/internal/catalog"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/models"
)

const albumReportKey = `CASE WHEN l.album_group_id>0 THEN 'group:'||l.album_group_id ELSE 'release:'||l.release_id END`

func albumReportQuery(query string, timeframe db.Timeframe) string {
	query = strings.ReplaceAll(query, "FROM listens l", "FROM displayed_listens l")
	if !includesUndated(timeframe) {
		return strings.ReplaceAll(query, "FROM displayed_listens l", "FROM album_report_events l")
	}
	query = countedListeningQuery(query, timeframe)
	query = strings.ReplaceAll(query, "FROM listening_counts l", "FROM album_report_counts l")
	return query
}

func (s *Sqlite) topAlbumReports(ctx context.Context, opts db.GetItemsOpts) (*db.PaginatedResponse[db.RankedItem[*models.Album]], error) {
	if opts.Limit == 0 {
		opts.Limit = defaultItemsPerPage
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	offset := (opts.Page - 1) * opts.Limit
	t1, t2 := db.TimeframeToTimeRange(opts.Timeframe)
	dates := `l.listened_at BETWEEN ? AND ?`
	if includesUndated(opts.Timeframe) {
		dates = `(l.listened_at IS NULL OR l.listened_at BETWEEN ? AND ?)`
	}
	artist := ""
	args := []any{t1.Unix(), t2.Unix()}
	if opts.ArtistID != 0 {
		artist = ` AND EXISTS(SELECT 1 FROM displayed_artist_tracks ar WHERE ar.track_id=l.track_id AND ar.artist_id=?)`
		args = append(args, opts.ArtistID)
	}
	if opts.Genre != "" {
		artist += ` AND EXISTS(SELECT 1 FROM displayed_artist_tracks gat JOIN displayed_artist_genres ag ON ag.artist_id=gat.artist_id WHERE gat.track_id=l.track_id AND ag.genre=?)`
		args = append(args, opts.Genre)
	}
	args = append(args, opts.Limit, offset)
	rows, err := s.db.QueryContext(ctx, `WITH counts AS (
 SELECT `+albumReportKey+` AS report_key,l.album_group_id,MIN(l.release_id) AS release_id,SUM(l.amount) AS listen_count
 FROM album_report_counts l WHERE `+dates+artist+` GROUP BY report_key
 ),ranked AS (SELECT *,RANK() OVER(ORDER BY listen_count DESC) AS rank,COUNT(*) OVER() AS total_count FROM counts ORDER BY listen_count DESC,report_key LIMIT ? OFFSET ?)
 SELECT COALESCE(g.canonical_release_id,c.release_id),COALESCE(g.title,(SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=r.id),r.title),NULLIF(COALESCE((SELECT identifier FROM catalog_identifiers ci WHERE ci.kind='album' AND ci.item_id=r.id),r.musicbrainz_id),''),r.image,COALESCE((SELECT various_artists FROM album_classification ac WHERE ac.album_id=r.id),r.various_artists),c.listen_count,c.rank,c.total_count,c.album_group_id
 FROM ranked c LEFT JOIN album_groups g ON g.id=c.album_group_id
 JOIN releases_with_title r ON r.id=COALESCE(g.canonical_release_id,c.release_id) ORDER BY c.rank,c.report_key`, args...)
	if err != nil {
		return nil, err
	}
	items := []db.RankedItem[*models.Album]{}
	var total int64
	for rows.Next() {
		a := &models.Album{}
		var mbz, image sql.NullString
		var various int
		var item db.RankedItem[*models.Album]
		if err = rows.Scan(&a.ID, &a.Title, &mbz, &image, &various, &a.ListenCount, &item.Rank, &total, &a.AlbumGroupID); err != nil {
			rows.Close()
			return nil, err
		}
		if a.AlbumGroupID == 0 {
			a.MbzID = parseNullableUUID(mbz)
		}
		a.Image = catalog.BuildImageList(parseNullableUUID(image))
		a.VariousArtists = various == 1
		item.Item = a
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		item.Item.Artists, err = s.artistsForRelease(ctx, item.Item.ID)
		if err != nil {
			return nil, err
		}
		if item.Item.AlbumGroupID > 0 {
			item.Item.Artists, err = s.albumGroupArtists(ctx, item.Item.AlbumGroupID, item.Item.Artists)
			if err != nil {
				return nil, err
			}
		}
	}
	return &db.PaginatedResponse[db.RankedItem[*models.Album]]{Items: items, TotalCount: total, ItemsPerPage: int32(opts.Limit), HasNextPage: int64(offset+len(items)) < total, CurrentPage: int32(opts.Page)}, nil
}

func (s *Sqlite) GetAlbumGroup(ctx context.Context, id int32) (*models.Album, error) {
	var releaseID int32
	var title string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_release_id,title FROM album_groups WHERE id=? AND EXISTS(SELECT 1 FROM album_group_members m WHERE m.group_id=album_groups.id)`, id).Scan(&releaseID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	album, err := s.getAlbumByID(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	album.AlbumGroupID = id
	album.Artists, err = s.albumGroupArtists(ctx, id, album.Artists)
	if err != nil {
		return nil, err
	}
	album.Title = title
	album.MbzID = nil
	if err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0),COALESCE(MIN(listened_at),0) FROM album_report_counts WHERE album_group_id=?`, id).Scan(&album.ListenCount, &album.FirstListen); err != nil {
		return nil, err
	}
	var firstKey string
	if err = s.db.QueryRowContext(ctx, `SELECT play_key FROM album_report_events WHERE album_group_id=? ORDER BY listened_at,play_key LIMIT 1`, id).Scan(&firstKey); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	album.FirstListenDate, err = s.firstListenCalendarDate(ctx, firstKey)
	if err != nil {
		return nil, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(t.duration),0) FROM album_report_events l JOIN tracks t ON t.id=l.track_id WHERE l.album_group_id=?`, id).Scan(&album.TimeListened); err != nil {
		return nil, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT rank FROM (SELECT album_group_id,RANK() OVER(ORDER BY SUM(amount) DESC) AS rank FROM album_report_counts l GROUP BY `+albumReportKey+`) WHERE album_group_id=?`, id).Scan(&album.AllTimeRank); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.id,COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=r.id),r.title),COALESCE((SELECT SUM(l.amount) FROM album_report_counts l WHERE l.album_group_id=m.group_id AND l.release_id=m.release_id),0) FROM album_group_members m JOIN releases_with_title r ON r.id=m.release_id WHERE m.group_id=? ORDER BY r.title,r.id`, id)
	if err != nil {
		return nil, err
	}
	album.Editions = []models.AlbumEdition{}
	for rows.Next() {
		var edition models.AlbumEdition
		if err = rows.Scan(&edition.ID, &edition.Title, &edition.ListenCount); err != nil {
			rows.Close()
			return nil, err
		}
		album.Editions = append(album.Editions, edition)
	}
	err = rows.Err()
	rows.Close()
	return album, err
}

func (s *Sqlite) albumGroupArtists(ctx context.Context, id int32, fallback []models.SimpleArtist) ([]models.SimpleArtist, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT group_key FROM album_groups WHERE id=?`, id).Scan(&raw); err != nil {
		return nil, err
	}
	var key []string
	if err := json.Unmarshal([]byte(raw), &key); err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return fallback, nil
	}
	credits := key[0]
	if len(key) == 3 && key[0] == "music" {
		var comparison []string
		if err := json.Unmarshal([]byte(key[1]), &comparison); err != nil {
			return nil, err
		}
		credits = comparison[0]
	}
	var ids []int32
	if err := json.Unmarshal([]byte("["+credits+"]"), &ids); err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return fallback, nil
	}
	if len(fallback) == 1 && fallback[0].ID == ids[0] {
		return fallback, nil
	}
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: ids[0]})
	if err != nil {
		return nil, err
	}
	return []models.SimpleArtist{{ID: artist.ID, Name: artist.Name}}, nil
}

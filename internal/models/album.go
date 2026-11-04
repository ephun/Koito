package models

import "github.com/google/uuid"

type AlbumEdition struct {
	ID          int32  `json:"id"`
	Title       string `json:"title"`
	ListenCount int64  `json:"listen_count"`
}

type Album struct {
	AlbumGroupID   int32          `json:"album_group_id,omitempty"`
	Editions       []AlbumEdition `json:"editions,omitempty"`
	ID             int32          `json:"id"`
	MbzID          *uuid.UUID     `json:"musicbrainz_id"`
	Title          string         `json:"title"`
	Image          ImageList      `json:"image"`
	Artists        []SimpleArtist `json:"artists"`
	VariousArtists bool           `json:"is_various_artists"`
	ListenCount    int64          `json:"listen_count"`
	TimeListened   int64          `json:"time_listened"`
	FirstListen    int64          `json:"first_listen"`
	AllTimeRank    int64          `json:"all_time_rank"`
}

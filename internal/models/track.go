package models

import "github.com/google/uuid"

type Track struct {
	FirstListenDate  string         `json:"first_listen_date,omitempty"`
	ScopeIssues      []FilterIssue  `json:"scope_issues,omitempty"`
	IdentifierIssues []FilterIssue  `json:"identifier_issues,omitempty"`
	NameIssues       []FilterIssue  `json:"name_issues,omitempty"`
	ID               int32          `json:"id"`
	MusicTrackID     int64          `json:"music_track_id,omitempty"`
	Albums           []TrackAlbum   `json:"albums,omitempty"`
	Recordings       []Recording    `json:"recordings,omitempty"`
	Title            string         `json:"title"`
	Artists          []SimpleArtist `json:"artists"`
	MbzID            *uuid.UUID     `json:"musicbrainz_id"`
	ListenCount      int64          `json:"listen_count"`
	Duration         int32          `json:"duration"`
	Image            ImageList      `json:"image"`
	AlbumID          int32          `json:"album_id"`
	TimeListened     int64          `json:"time_listened"`
	FirstListen      int64          `json:"first_listen"`
	AllTimeRank      int64          `json:"all_time_rank"`
}

type TrackAlbum struct {
	ID          int32  `json:"id"`
	Title       string `json:"title"`
	ListenCount int64  `json:"listen_count"`
}

type Recording struct {
	Key           string `json:"key"`
	MusicBrainzID string `json:"musicbrainz_id,omitempty"`
	SpotifyURI    string `json:"spotify_uri,omitempty"`
}

type SimpleTrack struct {
	ID      int32          `json:"id"`
	Title   string         `json:"title"`
	Artists []SimpleArtist `json:"artists"`
	Image   ImageList      `json:"image"`
}

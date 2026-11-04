package models

import "github.com/google/uuid"

type Artist struct {
	IdentifierIssues []FilterIssue   `json:"identifier_issues,omitempty"`
	NameIssues       []FilterIssue   `json:"name_issues,omitempty"`
	GenreIssues      []GenreIssue    `json:"genre_issues,omitempty"`
	Genres           []string        `json:"genres"`
	GenreEvidence    []GenreEvidence `json:"genre_evidence,omitempty"`
	ID               int32           `json:"id"`
	MbzID            *uuid.UUID      `json:"musicbrainz_id"`
	Name             string          `json:"name"`
	Aliases          []string        `json:"aliases"`
	Image            ImageList       `json:"image"`
	ListenCount      int64           `json:"listen_count"`
	TimeListened     int64           `json:"time_listened"`
	FirstListen      int64           `json:"first_listen"`
	IsPrimary        bool            `json:"is_primary,omitempty"`
	AllTimeRank      int64           `json:"all_time_rank"`
}

type GenreEvidence struct {
	RecordID int64  `json:"record_id"`
	SourceID int64  `json:"source_id"`
	Value    string `json:"value"`
	Field    string `json:"field"`
}

type ImageList struct {
	XS     string `json:"xs"`
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
	XL     string `json:"xl"`
}

type SimpleArtist struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type ArtistWithFullAliases struct {
	ID           int32      `json:"id"`
	MbzID        *uuid.UUID `json:"musicbrainz_id"`
	Name         string     `json:"name"`
	Aliases      []Alias    `json:"aliases"`
	Image        *uuid.UUID `json:"image"`
	ImageSource  string     `json:"image_source,omitempty"`
	ListenCount  int64      `json:"listen_count"`
	TimeListened int64      `json:"time_listened"`
	FirstListen  int64      `json:"first_listen"`
	IsPrimary    bool       `json:"is_primary,omitempty"`
}

type FilterIssue struct {
	FilterID string `json:"filter_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Reason   string `json:"reason"`
}

type GenreIssue = FilterIssue

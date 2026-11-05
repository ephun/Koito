package db

import (
	"time"

	"github.com/gabehf/koito/internal/models"
	"github.com/google/uuid"
)

type GetAlbumOpts struct {
	Original      bool
	ID            int32
	MusicBrainzID uuid.UUID
	ArtistID      int32
	Title         string
	Titles        []string
	Image         uuid.UUID
}

type GetArtistOpts struct {
	Original      bool
	ID            int32
	MusicBrainzID uuid.UUID
	Name          string
	Image         uuid.UUID
}

type GetTrackOpts struct {
	Original      bool
	ID            int32
	MusicBrainzID uuid.UUID
	Title         string
	ReleaseID     int32
	ArtistIDs     []int32
}

type SaveTrackOpts struct {
	Title          string
	AlbumID        int32
	ArtistIDs      []int32
	RecordingMbzID uuid.UUID
	Duration       int32
}

type SaveAlbumOpts struct {
	Title          string
	MusicBrainzID  uuid.UUID
	Type           string
	ArtistIDs      []int32
	VariousArtists bool
	Image          uuid.UUID
	ImageSrc       string
	Aliases        []string
}

type SaveArtistOpts struct {
	Name          string
	MusicBrainzID uuid.UUID
	Aliases       []string
	Image         uuid.UUID
	ImageSrc      string
}

type UpdateApiKeyLabelOpts struct {
	UserID int32
	ID     int32
	Label  string
}

type SaveUserOpts struct {
	Username string
	Password string
	Role     models.UserRole
}

type SaveApiKeyOpts struct {
	Key    string
	UserID int32
	Label  string
}

type SaveListenOpts struct {
	TrackID int32
	Time    time.Time
	UserID  int32
	Client  string
}

type UpdateTrackOpts struct {
	ID            int32
	MusicBrainzID uuid.UUID
	Duration      int32
	AddArtists    []int32
	RemoveArtists []int32
}

type UpdateArtistOpts struct {
	ID            int32
	MusicBrainzID uuid.UUID
	Image         uuid.UUID
	ImageSrc      string
}

type UpdateAlbumOpts struct {
	ID                   int32
	MusicBrainzID        uuid.UUID
	Image                uuid.UUID
	ImageSrc             string
	VariousArtistsUpdate bool
	VariousArtistsValue  bool
}

type UpdateUserOpts struct {
	ID       int32
	Username string
	Password string
}

type AddArtistsToAlbumOpts struct {
	AlbumID   int32
	ArtistIDs []int32
}

type AddArtistsToTrackOpts struct {
	TrackID   int32
	ArtistIDs []int32
}

type GetItemsOpts struct {
	Genre        string
	AlbumGroupID int
	Limit        int
	Page         int
	Timeframe    Timeframe

	// Used only for getting top tracks
	ArtistID int
	AlbumID  int

	// Used for getting listens
	TrackID int
}

type ListenActivityOpts struct {
	AlbumGroupID int32
	Step         StepInterval
	Range        int
	Month        int
	Year         int
	Timezone     *time.Location
	AlbumID      int32
	ArtistID     int32
	TrackID      int32
}

type TimeListenedOpts struct {
	AlbumGroupID int32
	Timeframe    Timeframe
	AlbumID      int32
	ArtistID     int32
	TrackID      int32
}

type GetExportPageOpts struct {
	UserID     int32
	ListenedAt time.Time
	TrackID    int32
	PlayKey    string
	Limit      int32
}

type GetInterestOpts struct {
	AlbumGroupID int32
	Buckets      int
	AlbumID      int32
	ArtistID     int32
	TrackID      int32
}

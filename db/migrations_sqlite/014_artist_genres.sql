-- +goose Up
CREATE TABLE artist_genres (
 artist_id INTEGER NOT NULL REFERENCES artists(id),
 genre TEXT NOT NULL,
 PRIMARY KEY(artist_id,genre)
);
CREATE INDEX artist_genres_genre ON artist_genres(genre,artist_id);
CREATE TABLE artist_genre_issues (
 artist_id INTEGER PRIMARY KEY REFERENCES artists(id),
 issues_json TEXT NOT NULL
);
-- +goose Down
DROP TABLE artist_genre_issues;
DROP TABLE artist_genres;

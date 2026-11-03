package sqlite_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/db/sqlite"
	exporter "github.com/gabehf/koito/internal/export"
	"github.com/gabehf/koito/internal/models"
)

func TestDistinctPlaysSurvivePaginationAndExport(t *testing.T) {
	s, err := sqlite.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	defer s.Close(ctx)
	err = s.Exec(`INSERT INTO users VALUES(1,'test','admin',x'00');
 INSERT INTO artists(id) VALUES(1);INSERT INTO artist_aliases VALUES(1,'Artist','user',1);
 INSERT INTO releases(id) VALUES(1);INSERT INTO release_aliases VALUES(1,'Album','user',1);
 INSERT INTO artist_releases VALUES(1,1,1);INSERT INTO tracks(id,release_id) VALUES(1,1);
 INSERT INTO track_aliases VALUES(1,'Song','user',1);INSERT INTO artist_tracks VALUES(1,1,1);`)
	if err != nil {
		t.Fatal(err)
	}
	const total = 1005
	for i := 0; i < total; i++ {
		if err = s.Exec(`INSERT INTO listens(track_id,listened_at,user_id,client,play_key) VALUES(1,1577934065,1,'sources:filters-v1',?)`, fmt.Sprintf("play-%04d", i)); err != nil {
			t.Fatal(err)
		}
	}
	cursor := db.GetExportPageOpts{UserID: 1, ListenedAt: time.Unix(0, 0), Limit: 1000}
	seen := map[string]bool{}
	for {
		page, e := s.GetExportPage(ctx, cursor)
		if e != nil {
			t.Fatal(e)
		}
		if len(page) == 0 {
			break
		}
		for _, item := range page {
			if seen[item.PlayKey] {
				t.Fatal("duplicate page row")
			}
			seen[item.PlayKey] = true
		}
		last := page[len(page)-1]
		cursor.ListenedAt = last.ListenedAt
		cursor.TrackID = last.TrackID
		cursor.PlayKey = last.PlayKey
	}
	if len(seen) != total {
		t.Fatal("export pagination lost same-second plays", len(seen))
	}
	var output bytes.Buffer
	if err = exporter.ExportData(ctx, &models.User{ID: 1, Username: "test"}, s, &output); err != nil {
		t.Fatal(err)
	}
	var exported exporter.KoitoExport
	if err = json.Unmarshal(output.Bytes(), &exported); err != nil || len(exported.Listens) != total {
		t.Fatal("export lost plays", len(exported.Listens), err)
	}
	page, err := s.GetListensPaginated(ctx, db.GetItemsOpts{Page: 1, Limit: 1000, Timeframe: db.Timeframe{Period: db.PeriodAllTime}})
	if err != nil || len(page.Items) != 1000 || page.Items[0].PlayKey == "" {
		t.Fatal("history lost play identity", err)
	}
	if err = s.DeleteListen(ctx, 1, time.Unix(1577934065, 0)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&n); err != nil || n != total {
		t.Fatal("legacy delete erased imported plays", n, err)
	}
}

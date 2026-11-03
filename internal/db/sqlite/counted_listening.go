package sqlite

import (
	"context"
	"strings"

	"github.com/gabehf/koito/internal/db"
)

func includesUndated(tf db.Timeframe) bool {
	return tf.Period == db.PeriodAllTime && tf.Year == 0 && tf.Month == 0 && tf.Week == 0 && tf.FromUnix == 0 && tf.ToUnix == 0 && tf.From.IsZero() && tf.To.IsZero()
}

// Only count/rank queries pass through this helper. Calendar and event queries
// keep using listens, so an undated amount cannot become a dated occurrence.
func countedListeningQuery(query string, tf db.Timeframe) string {
	if !includesUndated(tf) {
		return query
	}
	amount := "amount"
	if strings.Contains(query, "FROM listens l") {
		amount = "l.amount"
		query = strings.ReplaceAll(query, "l.listened_at BETWEEN ? AND ?", "(l.listened_at BETWEEN ? AND ? OR l.listened_at IS NULL)")
	} else {
		query = strings.ReplaceAll(query, "listened_at BETWEEN ? AND ?", "(listened_at BETWEEN ? AND ? OR listened_at IS NULL)")
	}
	query = strings.ReplaceAll(query, "FROM listens", "FROM listening_counts")
	query = strings.ReplaceAll(query, "COUNT(*) OVER ()", "__item_count__")
	query = strings.ReplaceAll(query, "COUNT(*)", "COALESCE(SUM("+amount+"),0)")
	return strings.ReplaceAll(query, "__item_count__", "COUNT(*) OVER ()")
}

func (s *Sqlite) CountUndatedListens(ctx context.Context, tf db.Timeframe) (int64, error) {
	if !includesUndated(tf) {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(undated_count),0) FROM source_aggregates`).Scan(&count)
	return count, err
}

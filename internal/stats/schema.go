package stats

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const (
	liveHistoryLen = 36
	bytesPerMB     = 1024 * 1024
	bytesPerGB     = 1024 * 1024 * 1024
)

// Store persists multi-granularity traffic metrics in SQLite and maintains a 36-point live rate buffer.
type Store struct {
	db       *sql.DB
	mu       sync.RWMutex
	liveDown []float64
	liveUp   []float64
}

// Open initializes the SQLite database at dbPath, creates required tables, and primes the live rate buffer.
func Open(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("stats: dbPath must not be empty")
	}

	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("stats: create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("stats: open sqlite: %w", err)
	}
	// SQLite allows a single writer at a time; limiting the pool prevents SQLITE_BUSY under concurrent writes.
	db.SetMaxOpenConns(1)

	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	liveDown, liveUp := defaultLiveWave()
	return &Store{
		db:       db,
		liveDown: liveDown,
		liveUp:   liveUp,
	}, nil
}

// Close releases the underlying SQLite connection pool.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA busy_timeout=5000;`,
		`PRAGMA synchronous=NORMAL;`,
		`CREATE TABLE IF NOT EXISTS traffic_samples (
			bucket_minute INTEGER NOT NULL,
			org_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			down_bytes INTEGER NOT NULL DEFAULT 0,
			up_bytes INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (bucket_minute, org_id, node_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_samples_minute ON traffic_samples(bucket_minute);`,
		`CREATE TABLE IF NOT EXISTS traffic_hourly (
			hour_key TEXT NOT NULL,
			org_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			down_bytes INTEGER NOT NULL DEFAULT 0,
			up_bytes INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (hour_key, org_id, node_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_hourly_key ON traffic_hourly(hour_key);`,
		`CREATE TABLE IF NOT EXISTS traffic_daily (
			day_key TEXT NOT NULL,
			org_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			down_bytes INTEGER NOT NULL DEFAULT 0,
			up_bytes INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (day_key, org_id, node_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_daily_key ON traffic_daily(day_key);`,
		`CREATE TABLE IF NOT EXISTS traffic_monthly (
			month_key TEXT NOT NULL,
			org_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			down_bytes INTEGER NOT NULL DEFAULT 0,
			up_bytes INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (month_key, org_id, node_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_monthly_key ON traffic_monthly(month_key);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("stats: init schema: %w", err)
		}
	}
	return nil
}

func defaultLiveWave() ([]float64, []float64) {
	down := make([]float64, liveHistoryLen)
	up := make([]float64, liveHistoryLen)
	for i := 0; i < liveHistoryLen; i++ {
		phase := float64(i) * 0.35
		d := 2.4 + 1.4*math.Sin(phase) + 0.5*math.Cos(phase*0.6)
		u := 0.4 + 0.2*math.Sin(phase+0.7)
		down[i] = round1(math.Max(0.2, d))
		up[i] = round1(math.Max(0.1, u))
	}
	return down, up
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func bytesToGB(b int64) float64 {
	return float64(b) / float64(bytesPerGB)
}

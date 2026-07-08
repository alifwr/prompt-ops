package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
	"aether-daemon/metrics"
)

var DB *sql.DB

// InitDB initializes the SQLite database connection and schemas
func InitDB(dbPath string) error {
	// Ensure the parent directory exists (e.g. /var/promptops/)
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	if err := DB.Ping(); err != nil {
		return err
	}

	query := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cpu REAL,
		ram_used INTEGER,
		ram_total INTEGER,
		disk_used INTEGER,
		disk_total INTEGER,
		timestamp TEXT
	);`

	_, err = DB.Exec(query)
	return err
}

// SaveMetrics writes a metrics snapshot to the database
func SaveMetrics(stats *metrics.SystemStats) error {
	query := `
	INSERT INTO metrics (cpu, ram_used, ram_total, disk_used, disk_total, timestamp)
	VALUES (?, ?, ?, ?, ?, ?);`

	_, err := DB.Exec(query, stats.CPUUsage, stats.RAMUsed, stats.RAMTotal, stats.DiskUsed, stats.DiskTotal, stats.Timestamp.Format(time.RFC3339))
	return err
}

// GetMetricsHistory retrieves historical metrics for the last N hours
func GetMetricsHistory(hours int) ([]metrics.SystemStats, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	query := `
	SELECT cpu, ram_used, ram_total, disk_used, disk_total, timestamp
	FROM metrics
	WHERE timestamp >= ?
	ORDER BY timestamp ASC;`

	rows, err := DB.Query(query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []metrics.SystemStats
	for rows.Next() {
		var stats metrics.SystemStats
		var tsStr string
		err := rows.Scan(&stats.CPUUsage, &stats.RAMUsed, &stats.RAMTotal, &stats.DiskUsed, &stats.DiskTotal, &tsStr)
		if err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, tsStr)
		if err == nil {
			stats.Timestamp = t
		}
		// Recalculate usage percentages
		if stats.RAMTotal > 0 {
			stats.RAMUsage = (float64(stats.RAMUsed) / float64(stats.RAMTotal)) * 100.0
		}
		if stats.DiskTotal > 0 {
			stats.DiskUsage = (float64(stats.DiskUsed) / float64(stats.DiskTotal)) * 100.0
		}
		history = append(history, stats)
	}
	return history, nil
}

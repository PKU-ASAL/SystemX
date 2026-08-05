package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/analytics/rarity"
	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/store"
)

func projectMetrics(ctx context.Context, db sqlExecutor, metrics store.Metrics) error {
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("encode metrics projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO metrics (tenant_id, metric_key, data)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, metric_key) DO UPDATE SET
  updated_at = now(),
  data = EXCLUDED.data
`, "default", "manager", data)
	if err != nil {
		return fmt.Errorf("project metrics: %w", err)
	}
	return nil
}

func (b *tableBackend) LoadMetrics(ctx context.Context) (store.Metrics, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryMetrics(ctx, b.db)
}

func (b *tableBackend) SaveMetrics(ctx context.Context, metrics store.Metrics) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return projectMetrics(ctx, b.db, metrics)
}

func (b *tableBackend) ResetMetrics(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return projectMetrics(ctx, b.db, store.Metrics{})
}

func queryMetrics(ctx context.Context, db sqlExecutor) (store.Metrics, error) {
	row := db.QueryRowContext(ctx, `
SELECT data FROM metrics
WHERE tenant_id = $1 AND metric_key = $2
`, "default", "manager")
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Metrics{}, nil
		}
		return store.Metrics{}, fmt.Errorf("query metrics: %w", err)
	}
	var metrics store.Metrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return store.Metrics{}, fmt.Errorf("decode metrics: %w", err)
	}
	return metrics, nil
}

func projectRarityBaseline(ctx context.Context, db sqlExecutor, baseline rarity.Baseline) error {
	for workload, signals := range baseline.WorkloadCounts {
		workload = strings.TrimSpace(workload)
		if workload == "" {
			workload = "global"
		}
		for signalName, count := range signals {
			signalName = strings.TrimSpace(signalName)
			if signalName == "" || count == 0 {
				continue
			}
			row := map[string]any{
				"workload_key": workload,
				"signal_name":  signalName,
				"signal_count": count,
			}
			data, err := json.Marshal(row)
			if err != nil {
				return fmt.Errorf("encode rarity baseline projection: %w", err)
			}
			_, err = db.ExecContext(ctx, `
INSERT INTO rarity_baseline (tenant_id, workload_key, signal_name, signal_count, data)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, workload_key, signal_name) DO UPDATE SET
  signal_count = EXCLUDED.signal_count,
  updated_at = now(),
  data = EXCLUDED.data
`, "default", workload, signalName, count, data)
			if err != nil {
				return fmt.Errorf("project rarity baseline: %w", err)
			}
		}
	}
	return nil
}

package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timmrkz/speakertrail/internal/settings"
)

// Pool spreads searches over its providers within their monthly budgets.
// Each search goes to the provider with the largest share of its budget
// left, so all of them are used together, and when one fails the next one
// answers. It is safe to use from several goroutines, and from several
// processes on the same database.
type Pool struct {
	DB        *pgxpool.Pool
	Providers []Provider
	// Now is the current time. Tests set it.
	Now func() time.Time
}

// Use is how much of a provider's budget this month is spent.
type Use struct {
	Provider string
	Used     int
	Budget   int
	// Set is true when the provider has an API key.
	Set bool
}

// Known are the providers the engine can use, with or without a key.
var Known = []string{"tavily", "brave"}

func (p *Pool) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// monthStart is the first moment of the current month in Berlin.
func (p *Pool) monthStart() time.Time {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		loc = time.UTC
	}
	t := p.now().In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
}

// Usage says how much of each known provider's budget is spent this month.
func (p *Pool) Usage(ctx context.Context) ([]Use, error) {
	cfg, err := settings.Load(ctx, p.DB)
	if err != nil {
		return nil, err
	}
	var out []Use
	for _, name := range Known {
		u := Use{Provider: name, Budget: cfg.Int(name+"_monthly_searches", 0)}
		u.Set = slices.ContainsFunc(p.Providers, func(pr Provider) bool { return pr.Name() == name })
		if err := p.DB.QueryRow(ctx, `SELECT count(*) FROM search_calls WHERE provider = $1 AND called_at >= $2`,
			name, p.monthStart()).Scan(&u.Used); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// Search runs a query on the provider with the most budget left, and on
// the next one when it fails. It returns the results and the provider
// that gave them.
func (p *Pool) Search(ctx context.Context, query string) ([]Result, string, error) {
	usage, err := p.Usage(ctx)
	if err != nil {
		return nil, "", err
	}
	left := map[string]float64{}
	for _, u := range usage {
		if u.Budget > 0 {
			left[u.Provider] = float64(u.Budget-u.Used) / float64(u.Budget)
		}
	}
	order := slices.Clone(p.Providers)
	slices.SortStableFunc(order, func(a, b Provider) int {
		switch la, lb := left[a.Name()], left[b.Name()]; {
		case la > lb:
			return -1
		case la < lb:
			return 1
		}
		return 0
	})
	var errs []error
	for _, pr := range order {
		id, ok, err := p.reserve(ctx, pr.Name(), query)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			continue
		}
		res, err := pr.Search(ctx, query)
		msg := ""
		if err != nil {
			msg = err.Error()
			errs = append(errs, err)
		}
		// Record the outcome even when the search was cancelled.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_, rerr := p.DB.Exec(rctx, `UPDATE search_calls SET results = $2, error = $3 WHERE id = $1`, id, len(res), msg)
		cancel()
		if rerr != nil {
			return nil, "", rerr
		}
		if err == nil {
			return res, pr.Name(), nil
		}
	}
	if len(errs) > 0 {
		return nil, "", errors.Join(errs...)
	}
	return nil, "", ErrNoBudget
}

// reserve counts a call against a provider's budget before it is made,
// in one short transaction that holds the provider's lock, so calls from
// several workers never go past the budget together. It reports false
// when the budget is spent.
func (p *Pool) reserve(ctx context.Context, provider, query string) (int64, bool, error) {
	var id int64
	err := pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('speakertrail:search:' || $1))`, provider); err != nil {
			return err
		}
		var budget, used int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = $1 || '_monthly_searches'), 0),
				(SELECT count(*) FROM search_calls WHERE provider = $1 AND called_at >= $2)`,
			provider, p.monthStart()).Scan(&budget, &used); err != nil {
			return err
		}
		if used >= budget {
			return nil
		}
		return tx.QueryRow(ctx, `INSERT INTO search_calls (provider, query, called_at) VALUES ($1, $2, $3) RETURNING id`,
			provider, query, p.now()).Scan(&id)
	})
	if err != nil {
		return 0, false, fmt.Errorf("search budget: %w", err)
	}
	return id, id != 0, nil
}

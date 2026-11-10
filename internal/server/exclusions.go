package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timmrkz/speakertrail/internal/pipeline"
	"github.com/timmrkz/speakertrail/internal/rubric"
)

// exclusion is one of Tim's words or phrases that leave a person out, with
// how many people found so far it hides.
type exclusion struct {
	ID     int64  `json:"id"`
	Phrase string `json:"phrase"`
	People int    `json:"people"`
}

// hidden finds the people an exclusion leaves out, by their headline and
// the roles their confirmed LinkedIn profile says they hold now. Whoever
// Tim connected with is never hidden. It tells how many each exclusion
// hides, the first that matches counting.
func (s *Server) hidden(ctx context.Context) (ids []int64, per map[string]int, err error) {
	phrases, err := pipeline.Exclusions(ctx, s.opts.Pool)
	if err != nil || len(phrases) == 0 {
		return []int64{}, map[string]int{}, err
	}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT p.id, p.headline || ' ' || COALESCE((SELECT string_agg(pr.current_roles, ' ') FROM profiles pr
			WHERE pr.person_id = p.id AND pr.platform = 'linkedin' AND pr.review = 'confirmed'), '')
		FROM people p WHERE NOT `+isConnected)
	if err != nil {
		return nil, nil, err
	}
	ids, per = []int64{}, map[string]int{}
	var id int64
	var text string
	_, err = pgx.ForEachRow(rows, []any{&id, &text}, func() error {
		if phrase, out := rubric.Excluded(text, phrases); out {
			ids = append(ids, id)
			per[phrase]++
		}
		return nil
	})
	return ids, per, err
}

func (s *Server) exclusions(w http.ResponseWriter, r *http.Request) {
	list, err := s.listExclusions(r.Context())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listExclusions(ctx context.Context) ([]exclusion, error) {
	_, per, err := s.hidden(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.opts.Pool.Query(ctx, `SELECT id, phrase FROM exclusions ORDER BY lower(phrase)`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (exclusion, error) {
		var x exclusion
		err := r.Scan(&x.ID, &x.Phrase)
		return x, err
	})
	for i := range list {
		list[i].People = per[list[i].Phrase]
	}
	return list, err
}

func (s *Server) addExclusion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phrase string `json:"phrase"`
	}
	if !decode(w, r, &body) {
		return
	}
	phrase := rubric.NormalPhrase(body.Phrase)
	if phrase == "" {
		fail(w, http.StatusBadRequest, "Enter a word or phrase, like Online Marketing")
		return
	}
	if len([]rune(phrase)) > 60 {
		fail(w, http.StatusBadRequest, "An exclusion is at most 60 characters")
		return
	}
	var id int64
	err := s.opts.Pool.QueryRow(r.Context(), `INSERT INTO exclusions (phrase) VALUES ($1) RETURNING id`, phrase).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		fail(w, http.StatusConflict, "This exclusion is already there")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	list, err := s.listExclusions(r.Context())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	for _, x := range list {
		if x.ID == id {
			writeJSON(w, http.StatusCreated, x)
			return
		}
	}
	s.internal(w, r, errors.New("the new exclusion is gone"))
}

func (s *Server) deleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tag, err := s.opts.Pool.Exec(r.Context(), `DELETE FROM exclusions WHERE id = $1`, id)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

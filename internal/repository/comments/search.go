package comments

import (
	"context"
	"fmt"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Search returns body matches in descending timestamp and ID order, with discussion roots.
// Blank queries are invalid; queries without searchable lexemes return an empty page.
func (r *Repository) Search(ctx context.Context, options domain.SearchOptions) (domain.SearchPage, error) {
	query, err := options.NormalizedQuery()
	if err != nil {
		return domain.SearchPage{}, err
	}
	args := []any{query, options.Limit}
	boundary := ""
	if options.After != nil {
		boundary = "AND (created_at, id) < ($3::timestamptz, $4::bigint)"
		args = append(args, options.After.CreatedAt, options.After.ID)
	}
	// Resolve ancestors only for returned matches, in the same snapshot as the search.
	sql := `WITH RECURSIVE candidates AS MATERIALIZED (
		SELECT id, parent_id, author, body, created_at FROM comments
		WHERE search_vector @@ plainto_tsquery('public.comment_search'::regconfig, $1) ` + boundary + `
		ORDER BY created_at DESC, id DESC LIMIT $2::bigint + 1
		), matches AS MATERIALIZED (
		SELECT * FROM candidates ORDER BY created_at DESC, id DESC LIMIT $2
		), ancestors AS (
		SELECT id AS match_id, id, parent_id FROM matches
		UNION ALL
		SELECT a.match_id, c.id, c.parent_id FROM comments c JOIN ancestors a ON c.id = a.parent_id
		)
		SELECT m.id, m.parent_id, m.author, m.body, m.created_at, a.id,
		(SELECT count(*) > $2 FROM candidates) AS has_next
		FROM matches m JOIN ancestors a ON a.match_id = m.id AND a.parent_id IS NULL
		ORDER BY m.created_at DESC, m.id DESC`
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return domain.SearchPage{}, fmt.Errorf("search comments: %w", mapError(ctx, err))
	}
	defer rows.Close()
	var page domain.SearchPage
	var hasNext bool
	for rows.Next() {
		var match domain.SearchMatch
		c := &match.Comment
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Author, &c.Body, &c.CreatedAt, &match.RootID, &hasNext); err != nil {
			return domain.SearchPage{}, fmt.Errorf("scan search match: %w", mapError(ctx, err))
		}
		page.Matches = append(page.Matches, match)
	}
	if err := rows.Err(); err != nil {
		return domain.SearchPage{}, fmt.Errorf("read search matches: %w", mapError(ctx, err))
	}
	if hasNext {
		last := page.Matches[len(page.Matches)-1].Comment
		page.Next = &domain.SearchCursor{CreatedAt: last.CreatedAt, ID: last.ID, Query: query}
	}
	return page, nil
}

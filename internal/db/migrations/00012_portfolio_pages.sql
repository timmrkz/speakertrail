-- Many portfolios link to a page of their own about each startup, not to
-- its website. That page is kept, and a lookup finds the website there.

-- +goose Up

ALTER TABLE organisations ADD COLUMN portfolio_page text NOT NULL DEFAULT '';
CREATE INDEX organisations_portfolio_page_idx ON organisations (portfolio_page) WHERE portfolio_page <> '';

-- +goose Down

DROP INDEX organisations_portfolio_page_idx;
ALTER TABLE organisations DROP COLUMN portfolio_page;

package database

import (
	"context"
	"database/sql"
	"fmt"
)

type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (q *Queries) BeginTx(ctx context.Context) (*sql.Tx, error) {
	b, ok := q.db.(txBeginner)
	if !ok {
		return nil, fmt.Errorf("underlying DBTX does not support starting transactions")
	}
	return b.BeginTx(ctx, nil)
}

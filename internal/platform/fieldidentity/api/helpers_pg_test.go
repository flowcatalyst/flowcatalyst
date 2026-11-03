//go:build integration

package api

import (
	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// usecaseTx wraps a raw pgx.Tx for the Persist/Consume repository methods.
func usecaseTx(tx pgx.Tx) *usecasepgx.DbTx { return usecasepgx.WrapTxForBootstrap(tx) }

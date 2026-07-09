package main

// db_tenant.go
//
// Tenant-scoped query wrapper for PostgreSQL Row-Level Security (RLS).
//
// Why this exists:
//   The `apexrmm_app` role (used by `db`) is NOSUPERUSER and NOBYPASSRLS, so
//   PostgreSQL RLS policies are enforced. Each policy compares
//   `tenant_id::text` to `current_setting('app.tenant_id')`. We set that
//   GUC at the start of every transaction so the policy lets through rows
//   for the caller's tenant and nothing else.
//
// Why two variants (Read / Write):
//   `WithTenantRead` rolls back the transaction at the end. SELECT-only
//   code never needs to persist, and the rollback makes it obvious at the
//   call site that no writes happened. `WithTenantWrite` commits, so
//   INSERT/UPDATE/DELETE persists. Naming makes intent explicit and
//   prevents the bug where someone wraps an INSERT in a Read wrapper and
//   is surprised that nothing was written.
//
// Why a callback, not a context value:
//   `*sql.Tx` is the type that holds the transaction. Callers that need
//   to do multiple statements share the same Tx (and thus the same
//   RLS context) automatically. We can't pass a Tx through context.Context
//   without changing every call site to extract it, which is more
//   invasive than a callback.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// WithTenantRead runs `fn` inside a transaction scoped to `tenantID`.
// The transaction is always rolled back — use this for SELECT-only flows.
// The RLS GUC `app.tenant_id` is set to `tenantID` via SET LOCAL, which
// is automatically cleared on commit/rollback.
func WithTenantRead(tenantID string, fn func(tx *sql.Tx) error) error {
	return withTenant(tenantID, fn, false)
}

// WithTenantWrite runs `fn` inside a transaction scoped to `tenantID` and
// commits the result on success. Use this for INSERT/UPDATE/DELETE flows.
func WithTenantWrite(tenantID string, fn func(tx *sql.Tx) error) error {
	return withTenant(tenantID, fn, true)
}

func withTenant(tenantID string, fn func(*sql.Tx) error, commit bool) error {
	if tenantID == "" {
		return errors.New("withTenant: empty tenantID")
	}
	if db == nil {
		return errors.New("withTenant: db pool is nil (initDB not called?)")
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("withTenant: BeginTx: %w", err)
	}
	// Belt-and-suspenders: if anything panics or we forget to Rollback
	// on an early return, the deferred Rollback still releases the conn.
	defer func() {
		_ = tx.Rollback()
	}()

	// SET LOCAL does NOT accept parameterized values (`SET LOCAL foo = $1`
	// is a syntax error). Use set_config(name, value, is_local) instead:
	// it is the function form of SET LOCAL, accepts $1, and the third
	// argument `true` makes the setting transaction-scoped (auto-cleared
	// on commit/rollback, identical semantics to SET LOCAL).
	if _, err := tx.Exec(`SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		return fmt.Errorf("withTenant: set_config app.tenant_id: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	if commit {
		return tx.Commit()
	}
	// Read path: explicit Rollback (the defer is a safety net).
	return tx.Rollback()
}



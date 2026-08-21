package database

import (
	"github.com/genesysflow/go-genesys/contracts"
	"github.com/genesysflow/go-genesys/query"
)

// TxScope pins ORM operations to a specific executor - typically a
// database transaction. Every package-level helper accepts an optional
// trailing scope, so writes inside a Transaction callback are
// all-or-nothing:
//
//	err := database.WithinTransaction(func(tx *database.TxScope) error {
//	    order := &Order{Total: 100}
//	    if err := database.Create(order, tx); err != nil {
//	        return err // rolls back
//	    }
//	    return database.AttachScoped(tx, order, "Tags", tag)
//	})
//
// Calling a helper without a scope uses the default connection, as
// before.
type TxScope struct {
	driver   string
	executor query.Executor
}

// NewScope pins ORM helpers to an arbitrary executor and driver -
// useful for transactions begun manually or non-default connections.
func NewScope(driver string, executor query.Executor) *TxScope {
	return &TxScope{driver: driver, executor: executor}
}

// Driver returns the scope's driver name.
func (s *TxScope) Driver() string { return s.driver }

// Executor returns the scope's executor.
func (s *TxScope) Executor() query.Executor { return s.executor }

// WithinTransaction begins a transaction on the default connection,
// runs fn with a scope bound to it, and commits - or rolls back when fn
// returns an error or panics. Laravel's DB::transaction for the ORM.
func WithinTransaction(fn func(tx *TxScope) error) error {
	conn := mustDefault().Connection()
	return conn.Transaction(func(tx contracts.Transaction) error {
		return fn(NewScope(conn.Driver(), tx))
	})
}

// executorFor resolves the driver and executor for an operation: the
// given scope when present, the model's connection otherwise.
func executorFor[T any](scope []*TxScope) (string, query.Executor) {
	if len(scope) > 0 && scope[0] != nil {
		return scope[0].driver, scope[0].executor
	}
	return connectionFor[T]()
}

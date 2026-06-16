package postgres

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
)

const FakeDriverName = "sysarmor-postgres-migrate-test"

func init() {
	sql.Register(FakeDriverName, fakeSQLDriver{})
}

var fakeSQLState struct {
	sync.Mutex
	lastQuery string
	execErr   error
}

func FakeSetExecError(err error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = ""
	fakeSQLState.execErr = err
}

func FakeLastQuery() string {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	return fakeSQLState.lastQuery
}

type fakeSQLDriver struct{}

func (fakeSQLDriver) Open(string) (driver.Conn, error) {
	return fakeSQLConn{}, nil
}

type fakeSQLConn struct{}

func (fakeSQLConn) Prepare(query string) (driver.Stmt, error) {
	return fakeSQLStmt{query: query}, nil
}

func (fakeSQLConn) Close() error {
	return nil
}

func (fakeSQLConn) Begin() (driver.Tx, error) {
	return fakeSQLTx{}, nil
}

type fakeSQLStmt struct {
	query string
}

func (s fakeSQLStmt) Close() error {
	return nil
}

func (s fakeSQLStmt) NumInput() int {
	return -1
}

func (s fakeSQLStmt) Exec([]driver.Value) (driver.Result, error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = s.query
	if fakeSQLState.execErr != nil {
		return nil, fakeSQLState.execErr
	}
	return driver.RowsAffected(1), nil
}

func (s fakeSQLStmt) Query([]driver.Value) (driver.Rows, error) {
	return fakeSQLRows{}, nil
}

type fakeSQLTx struct{}

func (fakeSQLTx) Commit() error {
	return nil
}

func (fakeSQLTx) Rollback() error {
	return nil
}

type fakeSQLRows struct{}

func (fakeSQLRows) Columns() []string {
	return nil
}

func (fakeSQLRows) Close() error {
	return nil
}

func (fakeSQLRows) Next([]driver.Value) error {
	return io.EOF
}

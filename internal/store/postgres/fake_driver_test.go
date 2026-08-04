package postgres

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
)

const FakeDriverName = "sysarmor-postgres-migrate-test"

func init() {
	sql.Register(FakeDriverName, fakeSQLDriver{})
}

var fakeSQLState struct {
	sync.Mutex
	lastQuery string
	queries   []string
	execErr   error
	queryErr  error
	queryRows [][]driver.Value
	commits   int
	rollbacks int
}

func FakeSetExecError(err error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = ""
	fakeSQLState.queries = nil
	fakeSQLState.execErr = err
	fakeSQLState.queryErr = nil
	fakeSQLState.queryRows = nil
	fakeSQLState.commits = 0
	fakeSQLState.rollbacks = 0
}

func FakeSetQueryResult(rows [][]byte, err error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = ""
	fakeSQLState.queries = nil
	fakeSQLState.execErr = nil
	fakeSQLState.queryErr = err
	fakeSQLState.queryRows = make([][]driver.Value, 0, len(rows))
	for _, row := range rows {
		fakeSQLState.queryRows = append(fakeSQLState.queryRows, []driver.Value{row})
	}
	fakeSQLState.commits = 0
	fakeSQLState.rollbacks = 0
}

func FakeTransactionCounts() (int, int) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	return fakeSQLState.commits, fakeSQLState.rollbacks
}

func FakeLastQuery() string {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	return fakeSQLState.lastQuery
}

func FakeAllQueries() string {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	return strings.Join(fakeSQLState.queries, "\n")
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
	fakeSQLState.queries = append(fakeSQLState.queries, s.query)
	if fakeSQLState.execErr != nil && !strings.Contains(s.query, "pg_advisory") {
		return nil, fakeSQLState.execErr
	}
	return driver.RowsAffected(1), nil
}

func (s fakeSQLStmt) Query([]driver.Value) (driver.Rows, error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = s.query
	fakeSQLState.queries = append(fakeSQLState.queries, s.query)
	if fakeSQLState.queryErr != nil {
		return nil, fakeSQLState.queryErr
	}
	rows := make([][]driver.Value, len(fakeSQLState.queryRows))
	for i := range fakeSQLState.queryRows {
		rows[i] = append([]driver.Value(nil), fakeSQLState.queryRows[i]...)
	}
	return &fakeSQLRows{rows: rows}, nil
}

type fakeSQLTx struct{}

func (fakeSQLTx) Commit() error {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.commits++
	return nil
}

func (fakeSQLTx) Rollback() error {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.rollbacks++
	return nil
}

type fakeSQLRows struct {
	rows [][]driver.Value
	next int
}

func (*fakeSQLRows) Columns() []string {
	return []string{"data"}
}

func (*fakeSQLRows) Close() error {
	return nil
}

func (r *fakeSQLRows) Next(dest []driver.Value) error {
	if r.next >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.next])
	r.next++
	return nil
}

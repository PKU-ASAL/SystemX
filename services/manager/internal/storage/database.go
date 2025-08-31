// internal/storage/database.go
package storage

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

type Database struct {
	db *sql.DB
}

func NewDatabase() (*Database, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &Database{db: db}, nil
}

func (d *Database) DB() *sql.DB {
	return d.db
}

func (d *Database) Close() error {
	return d.db.Close()
}

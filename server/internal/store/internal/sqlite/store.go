package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	// _ "github.com/mattn/go-sqlite3"
	"github.com/nxtcoder17/nxtpass/server/internal/store/models"
	"github.com/nxtcoder17/nxtpass/server/internal/ulid"

	_ "github.com/tursodatabase/go-libsql"
)

type Store struct {
	DB *sql.DB
}

var credstore_create_table = /*sql*/ `
CREATE TABLE IF NOT EXISTS credstore (
  id   BLOB  PRIMARY KEY,

  username text  NOT NULL,
  password text  NOT NULL,
  -- hosts will be stored as an []string
  hosts text,
  -- extras would be stored as a json object {"k1": "v1"}
  extras text,

  -- tags will be stored as an []string
  tags text,

	created_by text NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	deleted_at DATETIME
);
`

var activity_log_table_schema = /*sql*/ `
CREATE TABLE IF not exists activity_log (
  id   BLOB  PRIMARY KEY,
  timestamp INTEGER NOT NULL,
	sql_query text NOT NULL
);
`

func Connect(ctx context.Context, file string) (*sql.DB, error) {
	// db, err := sql.Open("sqlite3", file)
	db, err := sql.Open("libsql", fmt.Sprintf("file:./%s", file))
	if err != nil {
		return nil, errors.Join(errors.New("failed to open db"), err)
	}

	if _, err := db.ExecContext(ctx, credstore_create_table); err != nil {
		return nil, errors.Join(errors.New("failed to create credstore table"), err)
	}

	if _, err := db.ExecContext(ctx, activity_log_table_schema); err != nil {
		return nil, errors.Join(errors.New("failed to create credstore table"), err)
	}

	return db, err
}

var credential_create = SQLParse( /*sql*/ `
INSERT INTO credstore(id,username,password,hosts,extras,tags,created_by,created_at,updated_at)
VALUES(
	{{ .ID | squote }},
	{{ .Username | squote }},
	{{ .Password | squote }},
	json_array({{.Hosts | flatten }}),
	json_object({{.Extra | flatten }}),
	json_array({{.Tags | flatten }}),
	{{.CreatedBy | squote }},
	{{.CreatedAt }},
	{{.UpdatedAt }}
);
`)

var activity_log_entry = /*sql*/ `
INSERT INTO activity_log(id, timestamp, sql_query)
VALUES(?, ?, ?);
`

// Create implements store.Store.
func (s *Store) Create(ctx context.Context, cred models.Credential) (ulid.ID, error) {
	if cred.ID == "" {
		cred.ID = ulid.New()
	}

	credInsertQuery, err := SQLPrepare(credential_create, cred)
	if err != nil {
		return "", err
	}

	slog.Debug("create", "sql", credInsertQuery)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		tx.Rollback()
		return "", errors.Join(fmt.Errorf("failed to create transaction"), err)
	}

	if _, err := tx.ExecContext(ctx, credInsertQuery); err != nil {
		tx.Rollback()
		return "", errors.Join(fmt.Errorf("failed to execute credential create query"), err)
	}

	if _, err := tx.ExecContext(ctx, activity_log_entry, ulid.New(), cred.CreatedAt, credInsertQuery); err != nil {
		tx.Rollback()
		return "", errors.Join(fmt.Errorf("failed to execute activity_log insert query"), err)
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return "", errors.Join(fmt.Errorf("failed to commit transaction"), err)
	}

	slog.Debug("store.create", "cred.ID", cred.ID)
	return cred.ID, nil
}

// Delete implements store.Store.
func (s *Store) Delete(ctx context.Context, id ulid.ID) {
	panic("unimplemented")
}

// List implements store.Store.
func (s *Store) List(ctx context.Context, namespace string) {
	panic("unimplemented")
}

var last_checkpoint_event = SQLParse( /*sql*/ `
SELECT timestamp from activity_log
ORDER BY timestamp DESC 
LIMIT 1;
`)

func (s *Store) LastCheckpointAt(ctx context.Context) (int64, error) {
	s2, err := SQLPrepare(last_checkpoint_event, nil)
	if err != nil {
		return 0, err
	}

	rows, err := s.DB.QueryContext(ctx, s2)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if rows.Next() {
		var timestamp int64
		if err := rows.Scan(&timestamp); err != nil {
			return 0, err
		}

		slog.Debug("store.lastCheckpointAt", "timestamp", timestamp)
		return timestamp, nil
	}

	slog.Debug("store.lastCheckpointAt", "timestamp", 0)
	return 0, nil
}

var activity_log_since_timestamp = /*sql*/ `
SELECT timestamp, sql_query from activity_log
WHERE timestamp > ?
ORDER BY timestamp ASC;
`

func (s *Store) ChangeStream(ctx context.Context, since int64, writer io.Writer) error {
	start := time.Now()
	slog.Debug("(function call) store.ChangeStream", "since", since)
	defer func() {
		slog.Debug("(function call: finished) store.ChangeStream", "since", since, "took", fmt.Sprintf("%.2f", time.Since(start).Seconds()))
	}()
	rows, err := s.DB.QueryContext(ctx, activity_log_since_timestamp, since)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var actLog models.ActivityLog
		if err := rows.Scan(&actLog.Timestamp, &actLog.SQLQuery); err != nil {
			return err
		}

		slog.Debug("change stream", "row.timestamp", actLog.Timestamp, "row.query", actLog.SQLQuery)

		b, err := json.Marshal(actLog)
		if err != nil {
			return err
		}

		writer.Write(b)
		writer.Write([]byte{'\n'})
		writer.(http.Flusher).Flush()
	}

	return nil
}

func (s *Store) SyncRecord(ctx context.Context, timestamp int64, query string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to start transaction"))
	}

	if _, err := tx.ExecContext(ctx, query); err != nil {
		tx.Rollback()
		return err
	}

	if _, err := tx.ExecContext(ctx, activity_log_entry, ulid.New(), timestamp, query); err != nil {
		tx.Rollback()
		return errors.Join(fmt.Errorf("failed to execute activity_log insert query"), err)
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return errors.Join(fmt.Errorf("failed to commit transaction"), err)
	}

	return nil
}

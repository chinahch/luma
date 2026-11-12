package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

//go:embed migration2.sql
var migration2 string

//go:embed migration3.sql
var migration3 string

//go:embed migration4.sql
var migration4 string

var ErrConflict = errors.New("report identity conflicts with existing payload")
var ErrForbidden = errors.New("node is not active or authorized")
var ErrInvalid = errors.New("invalid accounting entry")

type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(74109914011)`); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT to_regclass('schema_version') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	version := 1
	if exists {
		if err = tx.QueryRow(ctx, `SELECT version FROM schema_version WHERE id=1`).Scan(&version); err != nil {
			return err
		}
		if version < 1 || version > 4 {
			return fmt.Errorf("unsupported database version %d", version)
		}
	} else {
		if _, err = tx.Exec(ctx, schema); err != nil {
			return err
		}
	}
	if version == 1 {
		if _, err = tx.Exec(ctx, migration2); err != nil {
			return err
		}
	}
	if version <= 2 {
		if _, err = tx.Exec(ctx, migration3); err != nil {
			return err
		}
	}
	if version <= 3 {
		if _, err = tx.Exec(ctx, migration4); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Hash(value string) []byte { h := sha256.Sum256([]byte(value)); return h[:] }

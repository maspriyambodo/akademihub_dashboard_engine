package db

import (
	"time"

	pgxv5 "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/sekolahpintar/dashboard-engine/internal/config"
)

func New(cfg *config.Config) (*sqlx.DB, error) {
	connConfig, err := pgxv5.ParseConfig(cfg.DB.DSN())
	if err != nil {
		return nil, err
	}
	// Simple protocol so pgbouncer transaction-pooling does not break on
	// cached prepared statements ("unnamed prepared statement does not exist").
	connConfig.DefaultQueryExecMode = pgxv5.QueryExecModeSimpleProtocol
	stdDB := stdlib.OpenDB(*connConfig)
	database := sqlx.NewDb(stdDB, "pgx")

	database.SetMaxOpenConns(30)
	database.SetMaxIdleConns(10)
	database.SetConnMaxLifetime(5 * time.Minute)
	database.SetConnMaxIdleTime(2 * time.Minute)

	if err := database.Ping(); err != nil {
		return nil, err
	}

	return database, nil
}

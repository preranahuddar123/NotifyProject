package db

import (
	"database/sql"
	"fmt"
	"log"

	"NotifyProject/config"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLDetailsSvc struct {
	cfg *config.Config
}

func NewMySQLDetailsSvc(cfg *config.Config) *MySQLDetailsSvc {
	return &MySQLDetailsSvc{
		cfg: cfg,
	}
}

func (m *MySQLDetailsSvc) ConnectMySQL() *sql.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local",
		m.cfg.MySQLDetails.Username,
		m.cfg.MySQLDetails.Password,
		m.cfg.MySQLDetails.Address,
		m.cfg.MySQLDetails.Port,
		m.cfg.MySQLDetails.DBName,
	)

	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}

	if err := conn.Ping(); err != nil {
		log.Fatalf("Failed to ping MySQL: %v", err)
	}

	log.Println("Connected to MySQL")
	return conn
}

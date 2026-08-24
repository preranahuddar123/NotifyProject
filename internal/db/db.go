package db

import (
	"NotifyProject/models"
	"database/sql"
	"fmt"
	"log"

	"NotifyProject/config"

	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type MySQLDetailsSvc struct {
	cfg *config.Config
}

func NewMySQLDetailsSvc(cfg *config.Config) *MySQLDetailsSvc {
	return &MySQLDetailsSvc{
		cfg: cfg,
	}
}

// dsnWithoutDB builds a connection string without the database name (used for DB creation).
func (m *MySQLDetailsSvc) dsnWithoutDB() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=True&loc=Local",
		m.cfg.MySQLDetails.Username,
		m.cfg.MySQLDetails.Password,
		m.cfg.MySQLDetails.Address,
		m.cfg.MySQLDetails.Port,
	)
}

// dsn builds the MySQL connection string.
func (m *MySQLDetailsSvc) dsn() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		m.cfg.MySQLDetails.Username,
		m.cfg.MySQLDetails.Password,
		m.cfg.MySQLDetails.Address,
		m.cfg.MySQLDetails.Port,
		m.cfg.MySQLDetails.DBName,
	)
}

// ensureDatabase creates the database if it does not exist.
func (m *MySQLDetailsSvc) ensureDatabase() {
	gormDB, err := gorm.Open(mysqldriver.Open(m.dsnWithoutDB()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("GORM: failed to connect to MySQL server: %v", err)
	}
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", m.cfg.MySQLDetails.DBName)
	if err := gormDB.Exec(sql).Error; err != nil {
		log.Fatalf("GORM: failed to create database %q: %v", m.cfg.MySQLDetails.DBName, err)
	}
	log.Printf("GORM: database %q ready", m.cfg.MySQLDetails.DBName)
}

// AutoMigrate creates the database if needed, then creates tables that don't exist yet.
func (m *MySQLDetailsSvc) AutoMigrate() {
	m.ensureDatabase()

	gormDB, err := gorm.Open(mysqldriver.Open(m.dsn()), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		log.Fatalf("GORM: failed to connect for migration: %v", err)
	}

	// Only create tables that don't already exist — never alter existing ones
	for _, model := range []interface{}{
		&models.LeadDetails{},
		&models.MeetingDetails{},
		&models.Successful{},
		&models.Booking{},
		&models.Notification{},
		&models.NotificationRecipient{},
		&models.DesignUserNotification{},
	} {
		if !gormDB.Migrator().HasTable(model) {
			if err := gormDB.Migrator().CreateTable(model); err != nil {
				log.Fatalf("GORM: CreateTable failed for %T: %v", model, err)
			}
			log.Printf("GORM: created table for %T", model)
		}
	}

	log.Println("GORM: AutoMigrate completed successfully")
}

// ConnectMySQL returns a *sql.DB used by the service layer for raw queries.
func (m *MySQLDetailsSvc) ConnectMySQL() *sql.DB {
	gormDB, err := gorm.Open(mysqldriver.Open(m.dsn()), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("Failed to ping MySQL: %v", err)
	}

	log.Println("Connected to MySQL")
	return sqlDB
}

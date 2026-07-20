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

// AutoMigrate creates tables that don't exist yet.
// It will NOT alter or touch tables that already exist in the DB.
func (m *MySQLDetailsSvc) AutoMigrate() {
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

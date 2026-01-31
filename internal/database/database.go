package database

import (
	"os"
	"path/filepath"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func New(cfg *config.Config) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(cfg.Database.Path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Server{},
		&models.Backup{},
		&models.BackupSchedule{},
		&models.Session{},
	); err != nil {
		return nil, err
	}

	return db, nil
}

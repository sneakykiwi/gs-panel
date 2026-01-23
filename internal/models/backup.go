package models

import (
	"time"

	"gorm.io/gorm"
)

type BackupType string

const (
	BackupTypeManual     BackupType = "manual"
	BackupTypeScheduled  BackupType = "scheduled"
	BackupTypePreInstall BackupType = "pre-install"
	BackupTypePreRestore BackupType = "pre-restore"
)

type Backup struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	ServerID    uint           `gorm:"index;not null" json:"server_id"`
	UUID        string         `gorm:"uniqueIndex;not null" json:"uuid"`
	Name        string         `gorm:"not null" json:"name"`
	Description string         `json:"description"`
	Filename    string         `gorm:"not null" json:"filename"`
	SizeBytes   int64          `json:"size_bytes"`
	Checksum    string         `gorm:"not null" json:"checksum"`
	Type        BackupType     `gorm:"not null" json:"type"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Server      Server         `gorm:"foreignKey:ServerID" json:"-"`
}

type BackupSchedule struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	ServerID  uint           `gorm:"index;not null" json:"server_id"`
	Name      string         `gorm:"not null" json:"name"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
	CronExpr  string         `gorm:"not null" json:"cron_expr"`
	KeepCount int            `gorm:"default:5" json:"keep_count"`
	KeepDays  int            `gorm:"default:7" json:"keep_days"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Server    Server         `gorm:"foreignKey:ServerID" json:"-"`
}

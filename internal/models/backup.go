package models

import (
	"time"

	"github.com/google/uuid"
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
	ID          string         `gorm:"primarykey;size:36" json:"id"`
	ServerID    string         `gorm:"index;not null;size:36" json:"server_id"`
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

func (b *Backup) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.Must(uuid.NewV7()).String()
	}
	return nil
}

type BackupSchedule struct {
	ID        string         `gorm:"primarykey;size:36" json:"id"`
	ServerID  string         `gorm:"index;not null;size:36" json:"server_id"`
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

func (bs *BackupSchedule) BeforeCreate(tx *gorm.DB) error {
	if bs.ID == "" {
		bs.ID = uuid.Must(uuid.NewV7()).String()
	}
	return nil
}

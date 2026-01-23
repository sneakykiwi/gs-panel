package models

import (
	"time"

	"gorm.io/gorm"
)

type ServerStatus string

const (
	ServerStatusStopped  ServerStatus = "stopped"
	ServerStatusRunning  ServerStatus = "running"
	ServerStatusStarting ServerStatus = "starting"
)

type Server struct {
	ID          uint             `gorm:"primarykey" json:"id"`
	UUID        string           `gorm:"uniqueIndex;not null" json:"uuid"`
	Name        string           `gorm:"not null" json:"name"`
	GameType    string           `gorm:"not null" json:"game_type"`
	DockerImage string           `gorm:"not null" json:"docker_image"`
	ContainerID string           `json:"container_id"`
	Port        int              `json:"port"`
	MemoryLimit int              `json:"memory_limit"`
	Status      ServerStatus     `gorm:"default:stopped" json:"status"`
	Environment string           `gorm:"type:text" json:"-"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	DeletedAt   gorm.DeletedAt   `gorm:"index" json:"-"`
	Users       []User           `gorm:"many2many:server_users;" json:"-"`
	Backups     []Backup         `gorm:"foreignKey:ServerID" json:"-"`
	Schedules   []BackupSchedule `gorm:"foreignKey:ServerID" json:"-"`
}

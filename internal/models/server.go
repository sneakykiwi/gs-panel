package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServerStatus string

const (
	ServerStatusStopped  ServerStatus = "stopped"
	ServerStatusRunning  ServerStatus = "running"
	ServerStatusStarting ServerStatus = "starting"
)

type Server struct {
	ID                string           `gorm:"primarykey;size:36" json:"id"`
	Name              string           `gorm:"not null" json:"name"`
	GameType          string           `gorm:"not null" json:"game_type"`
	DockerImage       string           `gorm:"not null" json:"docker_image"`
	ContainerID       string           `json:"container_id"`
	Port              int              `json:"port"`
	MemoryLimit       int              `json:"memory_limit"`
	Status            ServerStatus     `gorm:"default:stopped" json:"status"`
	Environment       string           `gorm:"type:text" json:"-"`
	CustomEnvironment string           `gorm:"type:text" json:"-"`
	TemplateVersion   string           `json:"template_version"`
	TemplateConfig    string           `gorm:"type:text" json:"-"` // Full template config stored as JSON
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	DeletedAt         gorm.DeletedAt   `gorm:"index" json:"-"`
	Users             []User           `gorm:"many2many:server_users;" json:"-"`
	Backups           []Backup         `gorm:"foreignKey:ServerID" json:"-"`
	Schedules         []BackupSchedule `gorm:"foreignKey:ServerID" json:"-"`
}

func (s *Server) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.Must(uuid.NewV7()).String()
	}
	return nil
}

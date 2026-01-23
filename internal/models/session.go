package models

import (
	"time"

	"gorm.io/gorm"
)

type Session struct {
	ID        uint      `gorm:"primarykey"`
	Token     string    `gorm:"uniqueIndex;not null"`
	UserID    uint      `gorm:"index;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	User      User           `gorm:"foreignKey:UserID"`
}

func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

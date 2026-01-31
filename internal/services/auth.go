package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/models"

	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
)

type AuthService struct {
	db            *gorm.DB
	sessionExpiry time.Duration
}

func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{
		db:            db,
		sessionExpiry: 24 * time.Hour,
	}
}

func (s *AuthService) SetSessionExpiry(d time.Duration) {
	s.sessionExpiry = d
}

func (s *AuthService) Login(email, password string) (*models.Session, error) {
	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.CheckPassword(password) {
		return nil, ErrInvalidCredentials
	}

	token, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	session := &models.Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(s.sessionExpiry),
	}

	if err := s.db.Create(session).Error; err != nil {
		return nil, err
	}

	return session, nil
}

func (s *AuthService) ValidateSession(token string) (*models.User, error) {
	var session models.Session
	if err := s.db.Preload("User").Where("token = ?", token).First(&session).Error; err != nil {
		return nil, ErrSessionNotFound
	}

	if session.IsExpired() {
		s.db.Delete(&session)
		return nil, ErrSessionExpired
	}

	return &session.User, nil
}

func (s *AuthService) Logout(token string) error {
	return s.db.Where("token = ?", token).Delete(&models.Session{}).Error
}

func (s *AuthService) CreateUser(email, password string, isAdmin bool) (*models.User, error) {
	user := &models.User{
		Email:   email,
		IsAdmin: isAdmin,
	}

	if err := user.SetPassword(password); err != nil {
		return nil, err
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) GetUserByID(id string) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) ListUsers() ([]models.User, error) {
	var users []models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *AuthService) DeleteUser(id string) error {
	return s.db.Delete(&models.User{}, "id = ?", id).Error
}

func (s *AuthService) HasAdminUser() bool {
	var count int64
	s.db.Model(&models.User{}).Where("is_admin = ?", true).Count(&count)
	return count > 0
}

func (s *AuthService) ResetPassword(userID, newPassword string) error {
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}

	if err := user.SetPassword(newPassword); err != nil {
		return err
	}

	if err := s.db.Save(&user).Error; err != nil {
		return err
	}

	return s.db.Where("user_id = ?", userID).Delete(&models.Session{}).Error
}

func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

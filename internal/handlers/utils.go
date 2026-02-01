package handlers

import (
	"errors"
	"path/filepath"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/models"
	"github.com/sneakykiwi/gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

func GetServerID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if id == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}
	return id, nil
}

func GetUserID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if id == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	return id, nil
}

func GetBackupID(c fiber.Ctx) (string, error) {
	id := c.Params("backupId")
	if id == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "Invalid backup ID")
	}
	return id, nil
}

func GetServerWithAuth(c fiber.Ctx, serverService *services.ServerService) (*models.Server, error) {
	id, err := GetServerID(c)
	if err != nil {
		return nil, err
	}

	server, err := serverService.Get(id)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	return server, nil
}

func GetServerPathByID(serverService *services.ServerService, cfg *config.Config, serverID string) (string, error) {
	server, err := serverService.Get(serverID)
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg.Storage.Servers, server.ID), nil
}

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("Password must be at least 8 characters")
	}
	return nil
}

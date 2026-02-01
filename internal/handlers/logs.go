package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/services"
)

type LogHandler struct {
	serverService *services.ServerService
	logService    *services.LogService
}

func NewLogHandler(serverService *services.ServerService, logService *services.LogService) *LogHandler {
	return &LogHandler{serverService: serverService, logService: logService}
}

func (h *LogHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/servers/:id/logs", h.ListLogs)
	app.Get("/servers/:id/logs/:filename", h.GetLogFile)
	app.Get("/servers/:id/logs/:filename/download", h.DownloadLogFile)
}

type listLogsResponse struct {
	Logs []services.LogFileInfo `json:"logs"`
}

func (h *LogHandler) ListLogs(c fiber.Ctx) error {
	serverID := c.Params("id")

	user := middleware.GetUser(c)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
	}

	if !user.IsAdmin {
		ok, err := h.serverService.UserHasAccess(user.ID, serverID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to validate access")
		}
		if !ok {
			return fiber.NewError(fiber.StatusForbidden, "Forbidden")
		}
	}

	logs, err := h.logService.ListSavedLogs(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to list logs")
	}

	return c.JSON(listLogsResponse{Logs: logs})
}

func (h *LogHandler) GetLogFile(c fiber.Ctx) error {
	serverID := c.Params("id")
	filename := c.Params("filename")

	user := middleware.GetUser(c)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
	}

	if !user.IsAdmin {
		ok, err := h.serverService.UserHasAccess(user.ID, serverID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to validate access")
		}
		if !ok {
			return fiber.NewError(fiber.StatusForbidden, "Forbidden")
		}
	}

	content, err := h.logService.ReadLogFile(serverID, filename)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	return c.Type("text/plain").SendString(content)
}

func (h *LogHandler) DownloadLogFile(c fiber.Ctx) error {
	serverID := c.Params("id")
	filename := c.Params("filename")

	user := middleware.GetUser(c)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
	}

	if !user.IsAdmin {
		ok, err := h.serverService.UserHasAccess(user.ID, serverID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to validate access")
		}
		if !ok {
			return fiber.NewError(fiber.StatusForbidden, "Forbidden")
		}
	}

	filePath, err := h.logService.GetLogFilePath(serverID, filename)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	// Set filename with proper timestamp for download
	downloadName := filename
	c.Set("Content-Disposition", "attachment; filename=\""+downloadName+"\"")

	return c.Download(filePath)
}

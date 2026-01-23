package handlers

import (
	"gs-panel/internal/forms"
	"gs-panel/internal/middleware"
	"gs-panel/internal/models"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type BackupHandler struct {
	backupService    *services.BackupService
	serverService    *services.ServerService
	schedulerService *services.SchedulerService
}

func NewBackupHandler(backupService *services.BackupService, serverService *services.ServerService, schedulerService *services.SchedulerService) *BackupHandler {
	return &BackupHandler{backupService: backupService, serverService: serverService, schedulerService: schedulerService}
}

func (h *BackupHandler) List(c fiber.Ctx) error {
	serverID := c.Params("id")
	if serverID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	backups, err := h.backupService.ListForServer(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to list backups: "+err.Error())
	}

	return c.Render("partials/backup_list", fiber.Map{"Backups": backups, "ServerID": serverID})
}

func (h *BackupHandler) Create(c fiber.Ctx) error {
	serverID := c.Params("id")
	if serverID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	var form forms.CreateBackup
	_ = c.Bind().Form(&form)
	if form.Name == "" {
		form.Name = "Manual Backup"
	}

	backup, err := h.backupService.Create(serverID, form.Name, form.Description, models.BackupTypeManual)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, "Failed to create backup: "+err.Error())
	}

	return c.Render("partials/backup_item", fiber.Map{"Backup": backup})
}

func (h *BackupHandler) Delete(c fiber.Ctx) error {
	backupID := c.Params("backupId")
	if backupID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid backup ID")
	}
	if err := h.backupService.Delete(backupID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete backup: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BackupHandler) Restore(c fiber.Ctx) error {
	backupID := c.Params("backupId")
	if backupID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid backup ID")
	}
	if err := h.backupService.Restore(backupID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to restore backup: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BackupHandler) Progress(c fiber.Ctx) error {
	serverID := c.Params("id")
	if serverID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	progress := h.backupService.GetProgress(serverID)
	if progress == nil {
		return c.JSON(fiber.Map{"status": "idle"})
	}
	return c.JSON(progress)
}

func (h *BackupHandler) Verify(c fiber.Ctx) error {
	backupID := c.Params("backupId")
	if backupID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid backup ID")
	}

	valid, err := h.backupService.VerifyChecksum(backupID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to verify backup: "+err.Error())
	}
	return c.JSON(fiber.Map{"valid": valid})
}

func (h *BackupHandler) ListSchedules(c fiber.Ctx) error {
	serverID := c.Params("id")
	if serverID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	schedules, err := h.schedulerService.ListForServer(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to list schedules: "+err.Error())
	}

	server, _ := h.serverService.Get(serverID)
	return c.Render("servers/schedules", fiber.Map{
		"Title":     "Backup Schedules",
		"User":      middleware.GetUser(c),
		"Server":    server,
		"Schedules": schedules,
	})
}

func (h *BackupHandler) CreateSchedule(c fiber.Ctx) error {
	serverID := c.Params("id")
	if serverID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	var form forms.CreateSchedule
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	if _, err := h.schedulerService.CreateSchedule(serverID, form.Name, form.CronExpr, form.KeepCount, form.KeepDays); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Failed to create schedule: "+err.Error())
	}

	return c.Redirect().To("/servers/" + serverID + "/schedules")
}

func (h *BackupHandler) DeleteSchedule(c fiber.Ctx) error {
	scheduleID := c.Params("scheduleId")
	if scheduleID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid schedule ID")
	}
	if err := h.schedulerService.DeleteSchedule(scheduleID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete schedule: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BackupHandler) ToggleSchedule(c fiber.Ctx) error {
	scheduleID := c.Params("scheduleId")
	if scheduleID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid schedule ID")
	}

	var form forms.ToggleSchedule
	_ = c.Bind().Form(&form)

	if err := h.schedulerService.ToggleSchedule(scheduleID, form.Enabled == "true"); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to toggle schedule: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

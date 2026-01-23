package handlers

import (
	"strconv"

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
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	backups, err := h.backupService.ListForServer(uint(serverID))
	if err != nil {
		return err
	}

	return c.Render("partials/backup_list", fiber.Map{"Backups": backups, "ServerID": serverID})
}

func (h *BackupHandler) Create(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	var form forms.CreateBackup
	_ = c.Bind().Form(&form)
	if form.Name == "" {
		form.Name = "Manual Backup"
	}

	backup, err := h.backupService.Create(uint(serverID), form.Name, form.Description, models.BackupTypeManual)
	if err != nil {
		return c.Status(fiber.StatusConflict).SendString(err.Error())
	}

	return c.Render("partials/backup_item", fiber.Map{"Backup": backup})
}

func (h *BackupHandler) Delete(c fiber.Ctx) error {
	backupID, err := strconv.ParseUint(c.Params("backupId"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.backupService.Delete(uint(backupID)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BackupHandler) Restore(c fiber.Ctx) error {
	backupID, err := strconv.ParseUint(c.Params("backupId"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.backupService.Restore(uint(backupID)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BackupHandler) Progress(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	progress := h.backupService.GetProgress(uint(serverID))
	if progress == nil {
		return c.JSON(fiber.Map{"status": "idle"})
	}
	return c.JSON(progress)
}

func (h *BackupHandler) Verify(c fiber.Ctx) error {
	backupID, err := strconv.ParseUint(c.Params("backupId"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	valid, err := h.backupService.VerifyChecksum(uint(backupID))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.JSON(fiber.Map{"valid": valid})
}

func (h *BackupHandler) ListSchedules(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	schedules, err := h.schedulerService.ListForServer(uint(serverID))
	if err != nil {
		return err
	}

	server, _ := h.serverService.Get(uint(serverID))
	return c.Render("servers/schedules", fiber.Map{
		"Title":     "Backup Schedules",
		"User":      middleware.GetUser(c),
		"Server":    server,
		"Schedules": schedules,
	})
}

func (h *BackupHandler) CreateSchedule(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	var form forms.CreateSchedule
	if err := c.Bind().Form(&form); err != nil {
		return fiber.ErrBadRequest
	}

	if _, err = h.schedulerService.CreateSchedule(uint(serverID), form.Name, form.CronExpr, form.KeepCount, form.KeepDays); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	return c.Redirect().To("/servers/" + c.Params("id") + "/schedules")
}

func (h *BackupHandler) DeleteSchedule(c fiber.Ctx) error {
	scheduleID, err := strconv.ParseUint(c.Params("scheduleId"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.schedulerService.DeleteSchedule(uint(scheduleID)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BackupHandler) ToggleSchedule(c fiber.Ctx) error {
	scheduleID, err := strconv.ParseUint(c.Params("scheduleId"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	var form forms.ToggleSchedule
	_ = c.Bind().Form(&form)

	if err := h.schedulerService.ToggleSchedule(uint(scheduleID), form.Enabled == "true"); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

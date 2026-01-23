package handlers

import (
	"strconv"

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
	return &BackupHandler{
		backupService:    backupService,
		serverService:    serverService,
		schedulerService: schedulerService,
	}
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

	return c.Render("partials/backup_list", fiber.Map{
		"Backups":  backups,
		"ServerID": serverID,
	})
}

func (h *BackupHandler) Create(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	name := c.FormValue("name")
	if name == "" {
		name = "Manual Backup"
	}
	description := c.FormValue("description")

	backup, err := h.backupService.Create(uint(serverID), name, description, models.BackupTypeManual)
	if err != nil {
		return c.Status(fiber.StatusConflict).SendString(err.Error())
	}

	return c.Render("partials/backup_item", fiber.Map{
		"Backup": backup,
	})
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

	user := middleware.GetUser(c)
	server, _ := h.serverService.Get(uint(serverID))

	return c.Render("servers/schedules", fiber.Map{
		"Title":     "Backup Schedules",
		"User":      user,
		"Server":    server,
		"Schedules": schedules,
	})
}

func (h *BackupHandler) CreateSchedule(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	name := c.FormValue("name")
	cronExpr := c.FormValue("cron_expr")
	keepCount, _ := strconv.Atoi(c.FormValue("keep_count"))
	keepDays, _ := strconv.Atoi(c.FormValue("keep_days"))

	_, err = h.schedulerService.CreateSchedule(uint(serverID), name, cronExpr, keepCount, keepDays)
	if err != nil {
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

	enabled := c.FormValue("enabled") == "true"

	if err := h.schedulerService.ToggleSchedule(uint(scheduleID), enabled); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

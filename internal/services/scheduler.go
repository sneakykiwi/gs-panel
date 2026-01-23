package services

import (
	"log"
	"sync"
	"time"

	"gs-panel/internal/models"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type SchedulerService struct {
	db            *gorm.DB
	backupService *BackupService
	cron          *cron.Cron
	jobs          map[uint]cron.EntryID
	mu            sync.Mutex
}

func NewSchedulerService(db *gorm.DB, backupService *BackupService) *SchedulerService {
	s := &SchedulerService{
		db:            db,
		backupService: backupService,
		cron:          cron.New(),
		jobs:          make(map[uint]cron.EntryID),
	}
	s.cron.Start()
	return s
}

func (s *SchedulerService) LoadSchedules() error {
	var schedules []models.BackupSchedule
	if err := s.db.Where("enabled = ?", true).Find(&schedules).Error; err != nil {
		return err
	}

	for _, schedule := range schedules {
		s.AddSchedule(&schedule)
	}

	return nil
}

func (s *SchedulerService) AddSchedule(schedule *models.BackupSchedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, exists := s.jobs[schedule.ID]; exists {
		s.cron.Remove(existingID)
	}

	entryID, err := s.cron.AddFunc(schedule.CronExpr, func() {
		s.runScheduledBackup(schedule)
	})
	if err != nil {
		return err
	}

	s.jobs[schedule.ID] = entryID
	return nil
}

func (s *SchedulerService) RemoveSchedule(scheduleID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.jobs[scheduleID]; exists {
		s.cron.Remove(entryID)
		delete(s.jobs, scheduleID)
	}
}

func (s *SchedulerService) runScheduledBackup(schedule *models.BackupSchedule) {
	name := "Scheduled Backup - " + time.Now().Format("2006-01-02 15:04")

	backup, err := s.backupService.Create(schedule.ServerID, name, "Automatic scheduled backup", models.BackupTypeScheduled)
	if err != nil {
		log.Printf("Scheduled backup failed for server %d: %v", schedule.ServerID, err)
		return
	}

	s.cleanupOldBackups(schedule)

	log.Printf("Scheduled backup %d created for server %d", backup.ID, schedule.ServerID)
}

func (s *SchedulerService) cleanupOldBackups(schedule *models.BackupSchedule) {
	var backups []models.Backup
	s.db.Where("server_id = ? AND type = ?", schedule.ServerID, models.BackupTypeScheduled).
		Order("created_at DESC").
		Find(&backups)

	if schedule.KeepCount > 0 && len(backups) > schedule.KeepCount {
		for _, backup := range backups[schedule.KeepCount:] {
			s.backupService.Delete(backup.ID)
		}
	}

	if schedule.KeepDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -schedule.KeepDays)
		for _, backup := range backups {
			if backup.CreatedAt.Before(cutoff) {
				s.backupService.Delete(backup.ID)
			}
		}
	}
}

func (s *SchedulerService) CreateSchedule(serverID uint, name, cronExpr string, keepCount, keepDays int) (*models.BackupSchedule, error) {
	if _, err := cron.ParseStandard(cronExpr); err != nil {
		return nil, err
	}

	schedule := &models.BackupSchedule{
		ServerID:  serverID,
		Name:      name,
		CronExpr:  cronExpr,
		Enabled:   true,
		KeepCount: keepCount,
		KeepDays:  keepDays,
	}

	if err := s.db.Create(schedule).Error; err != nil {
		return nil, err
	}

	s.AddSchedule(schedule)

	return schedule, nil
}

func (s *SchedulerService) DeleteSchedule(id uint) error {
	s.RemoveSchedule(id)
	return s.db.Delete(&models.BackupSchedule{}, id).Error
}

func (s *SchedulerService) ToggleSchedule(id uint, enabled bool) error {
	if err := s.db.Model(&models.BackupSchedule{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return err
	}

	if enabled {
		var schedule models.BackupSchedule
		if err := s.db.First(&schedule, id).Error; err != nil {
			return err
		}
		return s.AddSchedule(&schedule)
	}

	s.RemoveSchedule(id)
	return nil
}

func (s *SchedulerService) ListForServer(serverID uint) ([]models.BackupSchedule, error) {
	var schedules []models.BackupSchedule
	if err := s.db.Where("server_id = ?", serverID).Find(&schedules).Error; err != nil {
		return nil, err
	}
	return schedules, nil
}

func (s *SchedulerService) Stop() {
	s.cron.Stop()
}

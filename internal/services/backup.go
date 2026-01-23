package services

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gs-panel/internal/config"
	"gs-panel/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrBackupNotFound   = errors.New("backup not found")
	ErrBackupInProgress = errors.New("backup already in progress")
)

type BackupProgress struct {
	ServerID   uint
	BackupID   uint
	TotalFiles int
	Done       int
	Percentage float64
	Status     string
}

type BackupService struct {
	db       *gorm.DB
	cfg      *config.Config
	progress map[uint]*BackupProgress
}

func NewBackupService(db *gorm.DB, cfg *config.Config) *BackupService {
	if err := os.MkdirAll(cfg.Storage.Backups, 0755); err != nil {
		panic(err)
	}
	return &BackupService{
		db:       db,
		cfg:      cfg,
		progress: make(map[uint]*BackupProgress),
	}
}

func (s *BackupService) Create(serverID uint, name, description string, backupType models.BackupType) (*models.Backup, error) {
	if _, exists := s.progress[serverID]; exists {
		return nil, ErrBackupInProgress
	}

	var server models.Server
	if err := s.db.First(&server, serverID).Error; err != nil {
		return nil, err
	}

	backupUUID := uuid.New().String()
	filename := fmt.Sprintf("%s_%s.tar.gz", server.UUID, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(s.cfg.Storage.Backups, filename)

	backup := &models.Backup{
		ServerID:    serverID,
		UUID:        backupUUID,
		Name:        name,
		Description: description,
		Filename:    filename,
		Type:        backupType,
	}

	if err := s.db.Create(backup).Error; err != nil {
		return nil, err
	}

	s.progress[serverID] = &BackupProgress{
		ServerID: serverID,
		BackupID: backup.ID,
		Status:   "starting",
	}

	go s.runBackup(backup, &server, backupPath)

	return backup, nil
}

func (s *BackupService) runBackup(backup *models.Backup, server *models.Server, backupPath string) {
	defer delete(s.progress, server.ID)

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.UUID)

	var totalFiles int
	filepath.Walk(serverPath, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			totalFiles++
		}
		return nil
	})

	s.progress[server.ID].TotalFiles = totalFiles
	s.progress[server.ID].Status = "compressing"

	file, err := os.Create(backupPath)
	if err != nil {
		s.progress[server.ID].Status = "failed"
		return
	}
	defer file.Close()

	hash := sha256.New()
	multiWriter := io.MultiWriter(file, hash)
	gzWriter := gzip.NewWriter(multiWriter)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	var processed int
	err = filepath.Walk(serverPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(serverPath, path)

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(tarWriter, f); err != nil {
			return err
		}

		processed++
		s.progress[server.ID].Done = processed
		s.progress[server.ID].Percentage = float64(processed) / float64(totalFiles) * 100

		return nil
	})

	if err != nil {
		s.progress[server.ID].Status = "failed"
		os.Remove(backupPath)
		return
	}

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	stat, _ := os.Stat(backupPath)
	backup.SizeBytes = stat.Size()
	backup.Checksum = hex.EncodeToString(hash.Sum(nil))

	s.db.Save(backup)
	s.progress[server.ID].Status = "completed"
}

func (s *BackupService) Get(id uint) (*models.Backup, error) {
	var backup models.Backup
	if err := s.db.First(&backup, id).Error; err != nil {
		return nil, ErrBackupNotFound
	}
	return &backup, nil
}

func (s *BackupService) GetByUUID(uuid string) (*models.Backup, error) {
	var backup models.Backup
	if err := s.db.Where("uuid = ?", uuid).First(&backup).Error; err != nil {
		return nil, ErrBackupNotFound
	}
	return &backup, nil
}

func (s *BackupService) ListForServer(serverID uint) ([]models.Backup, error) {
	var backups []models.Backup
	if err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&backups).Error; err != nil {
		return nil, err
	}
	return backups, nil
}

func (s *BackupService) Delete(id uint) error {
	backup, err := s.Get(id)
	if err != nil {
		return err
	}

	backupPath := filepath.Join(s.cfg.Storage.Backups, backup.Filename)
	os.Remove(backupPath)

	return s.db.Delete(backup).Error
}

func (s *BackupService) Restore(backupID uint) error {
	backup, err := s.Get(backupID)
	if err != nil {
		return err
	}

	var server models.Server
	if err := s.db.First(&server, backup.ServerID).Error; err != nil {
		return err
	}

	backupPath := filepath.Join(s.cfg.Storage.Backups, backup.Filename)
	serverPath := filepath.Join(s.cfg.Storage.Servers, server.UUID)

	file, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(serverPath, header.Name)

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()
	}

	return nil
}

func (s *BackupService) GetProgress(serverID uint) *BackupProgress {
	return s.progress[serverID]
}

func (s *BackupService) VerifyChecksum(id uint) (bool, error) {
	backup, err := s.Get(id)
	if err != nil {
		return false, err
	}

	backupPath := filepath.Join(s.cfg.Storage.Backups, backup.Filename)
	file, err := os.Open(backupPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}

	computed := hex.EncodeToString(hash.Sum(nil))
	return computed == backup.Checksum, nil
}

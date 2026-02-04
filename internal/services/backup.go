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

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/models"

	"gorm.io/gorm"
)

var (
	ErrBackupNotFound   = errors.New("backup not found")
	ErrBackupInProgress = errors.New("backup already in progress")
)

type BackupProgress struct {
	ServerID   string
	BackupID   string
	TotalFiles int
	Done       int
	Percentage float64
	Status     string
}

type BackupService struct {
	db       *gorm.DB
	cfg      *config.Config
	progress map[string]*BackupProgress
}

func NewBackupService(db *gorm.DB, cfg *config.Config) *BackupService {
	if err := os.MkdirAll(cfg.Storage.Backups, 0755); err != nil {
		panic(err)
	}
	return &BackupService{
		db:       db,
		cfg:      cfg,
		progress: make(map[string]*BackupProgress),
	}
}

func (s *BackupService) Create(serverID string, name, description string, backupType models.BackupType) (*models.Backup, error) {
	if _, exists := s.progress[serverID]; exists {
		return nil, ErrBackupInProgress
	}

	var server models.Server
	if err := s.db.First(&server, "id = ?", serverID).Error; err != nil {
		return nil, err
	}

	filename := fmt.Sprintf("%s_%s.tar.gz", serverID, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(s.cfg.Storage.Backups, filename)

	backup := &models.Backup{
		ServerID:    serverID,
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
	serverID := server.ID
	defer delete(s.progress, serverID)

	serverPath := filepath.Join(s.cfg.Storage.Servers, serverID)

	var totalFiles int
	filepath.Walk(serverPath, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			totalFiles++
		}
		return nil
	})

	s.progress[serverID].TotalFiles = totalFiles
	s.progress[serverID].Status = "compressing"

	file, err := os.Create(backupPath)
	if err != nil {
		s.progress[serverID].Status = "failed"
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
		if err != nil {
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

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tarWriter, f); err != nil {
				return err
			}

			processed++
			s.progress[serverID].Done = processed
			s.progress[serverID].Percentage = float64(processed) / float64(totalFiles) * 100
		}

		return nil
	})

	if err != nil {
		s.progress[serverID].Status = "failed"
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
	s.progress[serverID].Status = "completed"
}

func (s *BackupService) Get(id string) (*models.Backup, error) {
	var backup models.Backup
	if err := s.db.First(&backup, "id = ?", id).Error; err != nil {
		return nil, ErrBackupNotFound
	}
	return &backup, nil
}

func (s *BackupService) ListForServer(serverID string) ([]models.Backup, error) {
	var backups []models.Backup
	if err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&backups).Error; err != nil {
		return nil, err
	}
	return backups, nil
}

func (s *BackupService) Delete(id string) error {
	backup, err := s.Get(id)
	if err != nil {
		return err
	}

	backupPath := filepath.Join(s.cfg.Storage.Backups, backup.Filename)
	os.Remove(backupPath)

	return s.db.Delete(backup).Error
}

func (s *BackupService) Restore(backupID string) error {
	backup, err := s.Get(backupID)
	if err != nil {
		return err
	}

	var server models.Server
	if err := s.db.First(&server, "id = ?", backup.ServerID).Error; err != nil {
		return err
	}

	backupPath := filepath.Join(s.cfg.Storage.Backups, backup.Filename)
	serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)

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

func (s *BackupService) GetProgress(serverID string) *BackupProgress {
	return s.progress[serverID]
}

func (s *BackupService) VerifyChecksum(id string) (bool, error) {
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

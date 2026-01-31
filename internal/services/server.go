package services

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/models"

	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"gorm.io/gorm"
)

var (
	ErrServerNotFound = errors.New("server not found")
	ErrServerRunning  = errors.New("server is running")
)

type ServerService struct {
	db        *gorm.DB
	docker    *client.Client
	cfg       *config.Config
	templates *TemplateService
}

func NewServerService(db *gorm.DB, docker *client.Client, cfg *config.Config, templates *TemplateService) *ServerService {
	return &ServerService{
		db:        db,
		docker:    docker,
		cfg:       cfg,
		templates: templates,
	}
}

type CreateServerRequest struct {
	Name        string
	GameType    string
	MemoryLimit int
	Port        int
}

func (s *ServerService) Create(req CreateServerRequest) (*models.Server, error) {
	template, ok := s.templates.Get(req.GameType)
	if !ok {
		return nil, fmt.Errorf("unknown game type: %s", req.GameType)
	}

	if req.MemoryLimit <= 0 {
		req.MemoryLimit = template.DefaultMemory
	}
	if req.Port <= 0 {
		req.Port = template.DefaultPort
	}

	serverID := uuid.Must(uuid.NewV7()).String()
	serverPath := filepath.Join(s.cfg.Storage.Servers, serverID)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for k, v := range template.Environment {
		env[k] = strings.ReplaceAll(v, "{{MEMORY}}", strconv.Itoa(req.MemoryLimit))
	}

	server := &models.Server{
		ID:          serverID,
		Name:        req.Name,
		GameType:    req.GameType,
		DockerImage: template.DockerImage,
		Port:        req.Port,
		MemoryLimit: req.MemoryLimit,
		Status:      models.ServerStatusStopped,
		Environment: s.templates.EncodeEnvironment(env),
	}

	if err := s.db.Create(server).Error; err != nil {
		os.RemoveAll(serverPath)
		return nil, err
	}

	return server, nil
}

func (s *ServerService) Get(id string) (*models.Server, error) {
	var server models.Server
	if err := s.db.First(&server, "id = ?", id).Error; err != nil {
		return nil, ErrServerNotFound
	}
	return &server, nil
}

func (s *ServerService) List() ([]models.Server, error) {
	var servers []models.Server
	if err := s.db.Find(&servers).Error; err != nil {
		return nil, err
	}
	return servers, nil
}

func (s *ServerService) ListForUser(userID string) ([]models.Server, error) {
	var user models.User
	if err := s.db.Preload("Servers").First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}
	return user.Servers, nil
}

func (s *ServerService) Delete(id string) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	if server.Status == models.ServerStatusRunning {
		if err := s.Stop(id); err != nil {
			return err
		}
	}

	if server.ContainerID != "" {
		ctx := context.Background()
		s.docker.ContainerRemove(ctx, server.ContainerID, client.ContainerRemoveOptions{Force: true})
	}

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)
	os.RemoveAll(serverPath)

	return s.db.Delete(server).Error
}

func (s *ServerService) Start(id string) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	ctx := context.Background()

	if server.ContainerID == "" {
		containerID, err := s.createContainer(ctx, server)
		if err != nil {
			return err
		}
		server.ContainerID = containerID
		s.db.Save(server)
	}

	_, err = s.docker.ContainerStart(ctx, server.ContainerID, client.ContainerStartOptions{})
	if err != nil {
		return err
	}

	server.Status = models.ServerStatusRunning
	return s.db.Save(server).Error
}

func (s *ServerService) Stop(id string) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	if server.ContainerID == "" {
		return nil
	}

	ctx := context.Background()
	timeout := 30
	_, err = s.docker.ContainerStop(ctx, server.ContainerID, client.ContainerStopOptions{Timeout: &timeout})
	if err != nil {
		return err
	}

	server.Status = models.ServerStatusStopped
	return s.db.Save(server).Error
}

func (s *ServerService) Restart(id string) error {
	if err := s.Stop(id); err != nil {
		return err
	}
	return s.Start(id)
}

func (s *ServerService) createContainer(ctx context.Context, server *models.Server) (string, error) {
	pullResp, err := s.docker.ImagePull(ctx, server.DockerImage, client.ImagePullOptions{})
	if err != nil {
		return "", err
	}
	pullResp.Close()

	env := s.templates.DecodeEnvironment(server.Environment)
	envList := make([]string, 0, len(env))
	for k, v := range env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)
	serverPath = filepath.Clean(serverPath)
	if !filepath.IsAbs(serverPath) {
		abs, err := filepath.Abs(serverPath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve server path: %w", err)
		}
		serverPath = abs
	}
	if _, err := os.Stat(serverPath); err != nil {
		return "", fmt.Errorf("server data directory not found: %s (%w)", serverPath, err)
	}

	tcpPort := network.MustParsePort(fmt.Sprintf("%d/tcp", server.Port))
	udpPort := network.MustParsePort(fmt.Sprintf("%d/udp", server.Port))
	portStr := fmt.Sprintf("%d", server.Port)

	containerConfig := &container.Config{
		Image: server.DockerImage,
		Env:   envList,
		ExposedPorts: network.PortSet{
			tcpPort: {},
			udpPort: {},
		},
		Tty:          true,
		OpenStdin:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	hostConfig := &container.HostConfig{
		PortBindings: network.PortMap{
			tcpPort: []network.PortBinding{{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: portStr}},
			udpPort: []network.PortBinding{{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: portStr}},
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: serverPath,
				Target: "/data",
			},
		},
		Resources: container.Resources{
			Memory: int64(server.MemoryLimit) * 1024 * 1024,
		},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}

	networkConfig := &network.NetworkingConfig{}

	resp, err := s.docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:             fmt.Sprintf("gs-panel-%s", server.ID),
		Config:           containerConfig,
		HostConfig:       hostConfig,
		NetworkingConfig: networkConfig,
	})
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (s *ServerService) AssignUser(serverID, userID string) error {
	return s.db.Exec("INSERT OR IGNORE INTO server_users (server_id, user_id) VALUES (?, ?)", serverID, userID).Error
}

func (s *ServerService) UnassignUser(serverID, userID string) error {
	return s.db.Exec("DELETE FROM server_users WHERE server_id = ? AND user_id = ?", serverID, userID).Error
}

func (s *ServerService) SyncStatus(id string) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	if server.ContainerID == "" {
		return nil
	}

	ctx := context.Background()
	result, err := s.docker.ContainerInspect(ctx, server.ContainerID, client.ContainerInspectOptions{})
	if err != nil {
		server.ContainerID = ""
		server.Status = models.ServerStatusStopped
		return s.db.Save(server).Error
	}

	if result.Container.State.Running {
		server.Status = models.ServerStatusRunning
	} else {
		server.Status = models.ServerStatusStopped
	}

	return s.db.Save(server).Error
}

func (s *ServerService) SyncAllStatuses() error {
	servers, err := s.List()
	if err != nil {
		return err
	}

	for _, server := range servers {
		s.SyncStatus(server.ID)
	}

	return nil
}

func (s *ServerService) UserHasAccess(userID, serverID string) (bool, error) {
	var count int64
	// server_users is the join table configured by gorm many2many.
	if err := s.db.Table("server_users").Where("server_id = ? AND user_id = ?", serverID, userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type UpdateServerRequest struct {
	Name        string
	MemoryLimit int
	Port        int
}

func (s *ServerService) Update(id string, req UpdateServerRequest) error {
	// Get server
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	// Validate port conflicts if port is being changed
	if req.Port != server.Port {
		if err := s.ValidatePort(req.Port, id); err != nil {
			return err
		}
	}

	// Check if server is running
	isRunning := server.Status == models.ServerStatusRunning

	// Stop server if running (to apply new configuration)
	if isRunning {
		if err := s.Stop(id); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
	}

	// Update database
	server.Name = req.Name
	server.MemoryLimit = req.MemoryLimit
	server.Port = req.Port

	// Update environment variables with new memory limit
	env := s.templates.DecodeEnvironment(server.Environment)
	for k, v := range env {
		env[k] = strings.ReplaceAll(v, strconv.Itoa(server.MemoryLimit), strconv.Itoa(req.MemoryLimit))
	}
	server.Environment = s.templates.EncodeEnvironment(env)

	if err := s.db.Save(server).Error; err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	// Restart server if it was running
	if isRunning {
		if err := s.Start(id); err != nil {
			return fmt.Errorf("server updated but failed to restart: %w", err)
		}
	}

	return nil
}

func (s *ServerService) ValidatePort(port int, excludeServerID string) error {
	var count int64
	s.db.Model(&models.Server{}).
		Where("port = ? AND id != ? AND deleted_at IS NULL", port, excludeServerID).
		Count(&count)

	if count > 0 {
		return errors.New("port already in use by another server")
	}
	return nil
}

func (s *ServerService) HasAccess(userID, serverID string) bool {
	hasAccess, _ := s.UserHasAccess(userID, serverID)
	return hasAccess
}

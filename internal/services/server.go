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

	"gs-panel/internal/config"
	"gs-panel/internal/models"

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

	serverUUID := uuid.New().String()
	serverPath := filepath.Join(s.cfg.Storage.Servers, serverUUID)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for k, v := range template.Environment {
		env[k] = strings.ReplaceAll(v, "{{MEMORY}}", strconv.Itoa(req.MemoryLimit))
	}

	server := &models.Server{
		UUID:        serverUUID,
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

func (s *ServerService) Get(id uint) (*models.Server, error) {
	var server models.Server
	if err := s.db.First(&server, id).Error; err != nil {
		return nil, ErrServerNotFound
	}
	return &server, nil
}

func (s *ServerService) GetByUUID(uuid string) (*models.Server, error) {
	var server models.Server
	if err := s.db.Where("uuid = ?", uuid).First(&server).Error; err != nil {
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

func (s *ServerService) ListForUser(userID uint) ([]models.Server, error) {
	var user models.User
	if err := s.db.Preload("Servers").First(&user, userID).Error; err != nil {
		return nil, err
	}
	return user.Servers, nil
}

func (s *ServerService) Delete(id uint) error {
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

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.UUID)
	os.RemoveAll(serverPath)

	return s.db.Delete(server).Error
}

func (s *ServerService) Start(id uint) error {
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

func (s *ServerService) Stop(id uint) error {
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

func (s *ServerService) Restart(id uint) error {
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

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.UUID)
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
		Name:             fmt.Sprintf("gs-panel-%s", server.UUID),
		Config:           containerConfig,
		HostConfig:       hostConfig,
		NetworkingConfig: networkConfig,
	})
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (s *ServerService) AssignUser(serverID, userID uint) error {
	return s.db.Exec("INSERT OR IGNORE INTO server_users (server_id, user_id) VALUES (?, ?)", serverID, userID).Error
}

func (s *ServerService) UnassignUser(serverID, userID uint) error {
	return s.db.Exec("DELETE FROM server_users WHERE server_id = ? AND user_id = ?", serverID, userID).Error
}

func (s *ServerService) SyncStatus(id uint) error {
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

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/logger"
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

type ErrLogSaveFailed struct {
	Err error
}

func (e *ErrLogSaveFailed) Error() string {
	return fmt.Sprintf("server stopped but logs could not be saved: %v", e.Err)
}

func (e *ErrLogSaveFailed) Unwrap() error {
	return e.Err
}

type ImagePullProgress struct {
	mu              sync.Mutex
	ServerID        string
	Image           string
	Status          string
	Error           string
	TotalLayers     int
	CompletedLayers int
	TotalBytes      int64
	DownloadedBytes int64
	ExtractedBytes  int64
	StartTime       time.Time
	LastUpdate      time.Time
	layerProgress   map[string]*layerProgress
}

type layerProgress struct {
	downloadedBytes int64
	totalBytes      int64
	extractedBytes  int64
	status          string
}

type ServerService struct {
	mu                sync.RWMutex
	db                *gorm.DB
	docker            *client.Client
	cfg               *config.Config
	templates         *TemplateService
	consoleService    *ConsoleService
	logService        *LogService
	imagePullProgress map[string]*ImagePullProgress
}

func NewServerService(db *gorm.DB, docker *client.Client, cfg *config.Config, templates *TemplateService, consoleService *ConsoleService, logService *LogService) *ServerService {
	return &ServerService{
		db:                db,
		docker:            docker,
		cfg:               cfg,
		templates:         templates,
		consoleService:    consoleService,
		logService:        logService,
		imagePullProgress: make(map[string]*ImagePullProgress),
	}
}

func (s *ServerService) getHostBasePath(ctx context.Context) (string, error) {
	// Quick check if we're even in Docker
	if _, err := os.Stat("/.dockerenv"); os.IsNotExist(err) {
		logger.Info().Msg("Not running in Docker (no /.dockerenv), using container paths directly")
		return "", nil
	}

	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to read /proc/self/mountinfo, falling back to native paths")
		return "", nil
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		destination := fields[9] // mount destination in container
		source := fields[8]      // host-side source path

		if destination == "/data" {
			logger.Info().Str("host_base_path", source).Msg("Successfully detected host path for /data")
			return source, nil
		}
	}

	logger.Warn().Msg("Could not find exact /data mount in /proc/self/mountinfo, falling back to native mode")
	return "", nil
}

func (s *ServerService) translatePathForDocker(ctx context.Context, containerPath string) (string, error) {
	hostBase, err := s.getHostBasePath(ctx)
	if err != nil {
		return "", err
	}

	if hostBase == "" || !strings.HasPrefix(containerPath, "/data") {
		logger.Info().Str("path", containerPath).Msg("Path translation skipped (native mode or not under /data)")
		return containerPath, nil
	}

	relativePath := strings.TrimPrefix(containerPath, "/data")
	hostPath := filepath.Join(hostBase, relativePath)

	logger.Info().
		Str("container_path", containerPath).
		Str("host_path", hostPath).
		Msg("Translated path for Docker bind mount")

	return hostPath, nil
}

func (s *ServerService) CheckImageExists(ctx context.Context, image string) (bool, error) {
	_, err := s.docker.ImageInspect(ctx, image)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "no such image") || strings.Contains(errStr, "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *ServerService) GetImagePullProgress(serverID string) *ImagePullProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.imagePullProgress[serverID]
}

func (s *ServerService) StartImagePull(server *models.Server, progress *ImagePullProgress) error {
	ctx := context.Background()

	progress.mu.Lock()
	progress.Status = "pulling"
	progress.StartTime = time.Now()
	progress.LastUpdate = time.Now()
	progress.layerProgress = make(map[string]*layerProgress)
	progress.mu.Unlock()

	s.broadcastPullProgress(server.ID, progress)

	if s.consoleService != nil {
		s.consoleService.Broadcast(server.ID, fmt.Sprintf("[info] Pulling Docker image: %s\n", server.DockerImage))
	}

	resp, err := s.docker.ImagePull(ctx, server.DockerImage, client.ImagePullOptions{})
	if err != nil {
		progress.mu.Lock()
		progress.Status = "error"
		progress.Error = err.Error()
		progress.mu.Unlock()

		s.broadcastPullProgress(server.ID, progress)

		if s.consoleService != nil {
			s.consoleService.Broadcast(server.ID, fmt.Sprintf("[error] Failed to pull image: %v\n", err))
		}
		return err
	}
	defer resp.Close()

	decoder := json.NewDecoder(resp)
	for {
		var msg json.RawMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			logger.Error().Str("server_id", server.ID).Err(err).Msg("Failed to decode pull progress")
			break
		}

		var pullMsg struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			Error          string `json:"error"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
		}

		if err := json.Unmarshal(msg, &pullMsg); err != nil {
			continue
		}

		if pullMsg.Error != "" {
			progress.mu.Lock()
			progress.Status = "error"
			progress.Error = pullMsg.Error
			progress.mu.Unlock()
			s.broadcastPullProgress(server.ID, progress)
			break
		}

		progress.mu.Lock()

		switch pullMsg.Status {
		case "Pulling fs layer":
			if pullMsg.ID != "" {
				progress.TotalLayers++
				progress.layerProgress[pullMsg.ID] = &layerProgress{
					status: "downloading",
				}
			}
		case "Downloading":
			if pullMsg.ID != "" {
				if layer, exists := progress.layerProgress[pullMsg.ID]; exists {
					if pullMsg.ProgressDetail.Total > 0 && layer.totalBytes == 0 {
						progress.TotalBytes += pullMsg.ProgressDetail.Total
						layer.totalBytes = pullMsg.ProgressDetail.Total
					}
					layer.downloadedBytes = pullMsg.ProgressDetail.Current
					layer.status = "downloading"
				}
			}
		case "Download complete":
			if pullMsg.ID != "" {
				if layer, exists := progress.layerProgress[pullMsg.ID]; exists {
					layer.status = "extracting"
					progress.CompletedLayers++
				}
			}
		case "Extracting":
			if pullMsg.ID != "" {
				if layer, exists := progress.layerProgress[pullMsg.ID]; exists {
					layer.extractedBytes = pullMsg.ProgressDetail.Current
				}
			}
		case "Pull complete":
			if pullMsg.ID != "" {
				if layer, exists := progress.layerProgress[pullMsg.ID]; exists {
					layer.status = "complete"
					if layer.totalBytes > 0 {
						progress.ExtractedBytes += layer.totalBytes
					}
				}
			}
		}

		progress.DownloadedBytes = 0
		for _, layer := range progress.layerProgress {
			progress.DownloadedBytes += layer.downloadedBytes
		}

		progress.LastUpdate = time.Now()
		progress.mu.Unlock()

		s.broadcastPullProgress(server.ID, progress)
	}

	progress.mu.Lock()
	progress.Status = "completed"
	progress.mu.Unlock()

	s.broadcastPullProgress(server.ID, progress)

	if s.consoleService != nil {
		s.consoleService.Broadcast(server.ID, "[info] Docker image pull completed successfully\n")
	}

	return nil
}

func (s *ServerService) broadcastPullProgress(serverID string, progress *ImagePullProgress) {
	if progress == nil {
		return
	}

	progress.mu.Lock()
	elapsed := time.Since(progress.StartTime).Seconds()
	downloadSpeed := float64(0)
	if elapsed > 0 && progress.DownloadedBytes > 0 {
		downloadSpeed = float64(progress.DownloadedBytes) / elapsed / (1024 * 1024)
	}

	remainingBytes := progress.TotalBytes - progress.DownloadedBytes
	eta := float64(0)
	if downloadSpeed > 0 && remainingBytes > 0 {
		eta = float64(remainingBytes) / (downloadSpeed * 1024 * 1024)
	}

	percentage := float64(0)
	if progress.TotalBytes > 0 {
		percentage = float64(progress.DownloadedBytes) / float64(progress.TotalBytes) * 100
	}

	payload := map[string]interface{}{
		"status":           progress.Status,
		"error":            progress.Error,
		"percentage":       percentage,
		"downloaded_bytes": progress.DownloadedBytes,
		"total_bytes":      progress.TotalBytes,
		"completed_layers": progress.CompletedLayers,
		"total_layers":     progress.TotalLayers,
		"speed_mbps":       downloadSpeed,
		"eta_seconds":      eta,
		"image":            progress.Image,
	}
	progress.mu.Unlock()

	if s.consoleService != nil {
		data, _ := json.Marshal(map[string]interface{}{
			"type": "image_pull",
			"data": payload,
		})
		s.consoleService.Broadcast(serverID, "[json]"+string(data))
	}
}

func (s *ServerService) cleanupImagePullProgress(serverID string) {
	s.mu.Lock()
	delete(s.imagePullProgress, serverID)
	s.mu.Unlock()
}

func (s *ServerService) replaceTemplateVars(cmd string, server *models.Server, serverPath, backupPath, logsPath string) string {
	result := cmd
	result = strings.ReplaceAll(result, "{{MEMORY}}", strconv.Itoa(server.MemoryLimit))
	result = strings.ReplaceAll(result, "{{PORT}}", strconv.Itoa(server.Port))
	result = strings.ReplaceAll(result, "{{SERVER_ID}}", server.ID)
	result = strings.ReplaceAll(result, "{{SERVER_NAME}}", server.Name)
	result = strings.ReplaceAll(result, "{{SERVER_DIR}}", serverPath)
	result = strings.ReplaceAll(result, "{{BACKUP_DIR}}", backupPath)
	result = strings.ReplaceAll(result, "{{LOGS_DIR}}", logsPath)
	return result
}

func (s *ServerService) GetServerTemplate(server *models.Server) (GameTemplate, bool) {
	if server.TemplateConfig != "" {
		var template GameTemplate
		if err := json.Unmarshal([]byte(server.TemplateConfig), &template); err != nil {
			logger.Warn().Str("server_id", server.ID).Msg("Failed to parse stored template config, falling back to template files")
		} else {
			serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)
			if err := s.templates.ValidateTemplate(&template, serverPath); err != nil {
				logger.Warn().Str("server_id", server.ID).Err(err).Msg("Stored template config failed validation, falling back")
			} else {
				return template, true
			}
		}
	}

	template, ok := s.templates.Get(server.GameType)
	if ok {
		serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)
		if err := s.templates.ValidateTemplate(&template, serverPath); err != nil {
			logger.Warn().Str("server_id", server.ID).Err(err).Msg("Failed to validate template, not setting")
			return GameTemplate{}, false
		}
		s.SetServerTemplate(server, template)
		s.db.Save(server)
	}
	return template, ok
}

func (s *ServerService) SetServerTemplate(server *models.Server, template GameTemplate) error {
	data, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("failed to serialize template config: %w", err)
	}
	server.TemplateConfig = string(data)
	return nil
}

func (s *ServerService) GetServerTemplateYAML(server *models.Server) (string, error) {
	template, ok := s.GetServerTemplate(server)
	if !ok {
		return "", fmt.Errorf("no template config found for server")
	}
	return s.templates.TemplateToYAML(template), nil
}

type CreateServerRequest struct {
	Name        string
	GameType    string
	MemoryLimit int
	Port        int
	CustomVars  map[string]string
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

	if err := s.templates.ValidateTemplate(&template, serverPath); err != nil {
		os.RemoveAll(serverPath)
		return nil, err
	}

	proposedPorts := s.GetPortsFromTemplate(template, req.Port)
	if err := s.ValidatePorts(proposedPorts, ""); err != nil {
		os.RemoveAll(serverPath)
		return nil, err
	}

	env := make(map[string]string)
	for k, v := range template.Environment {
		v = strings.ReplaceAll(v, "{{MEMORY}}", strconv.Itoa(req.MemoryLimit))
		v = strings.ReplaceAll(v, "{{PORT}}", strconv.Itoa(req.Port))
		v = strings.ReplaceAll(v, "{{SERVER_ID}}", serverID)
		v = strings.ReplaceAll(v, "{{SERVER_NAME}}", req.Name)
		env[k] = v
	}

	var customEnvStr string
	if len(req.CustomVars) > 0 {
		customEnvStr = s.templates.EncodeEnvironment(req.CustomVars)
	}

	templateConfigData, err := json.Marshal(template)
	if err != nil {
		os.RemoveAll(serverPath)
		return nil, fmt.Errorf("failed to serialize template config: %w", err)
	}

	server := &models.Server{
		ID:                serverID,
		Name:              req.Name,
		GameType:          req.GameType,
		DockerImage:       template.DockerImage,
		Port:              req.Port,
		MemoryLimit:       req.MemoryLimit,
		Status:            models.ServerStatusStopped,
		Environment:       s.templates.EncodeEnvironment(env),
		CustomEnvironment: customEnvStr,
		TemplateVersion:   template.Version,
		TemplateConfig:    string(templateConfigData),
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
	if err := s.db.Save(server).Error; err != nil {
		return err
	}

	if s.consoleService != nil {
		s.consoleService.EnsureSession(server.ID, server.ContainerID)
	}

	return nil
}

func (s *ServerService) Stop(id string) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	if server.ContainerID == "" {
		return nil
	}

	template, _ := s.GetServerTemplate(server)

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)
	backupPath := s.cfg.Storage.Backups
	logsPath := s.cfg.Storage.Logs

	if s.consoleService != nil && server.Status == models.ServerStatusRunning {
		if template.SaveCommand != "" {
			cmd := s.replaceTemplateVars(template.SaveCommand, server, serverPath, backupPath, logsPath)
			s.consoleService.SendCommand(server.ID, cmd)
			time.Sleep(2 * time.Second)
		}
		if template.StopCommand != "" {
			cmd := s.replaceTemplateVars(template.StopCommand, server, serverPath, backupPath, logsPath)
			s.consoleService.SendCommand(server.ID, cmd)
			time.Sleep(3 * time.Second)
		}
		s.consoleService.StopSession(server.ID)
	}

	var logSaveErr error
	ctx := context.Background()
	if s.logService != nil && server.ContainerID != "" {
		if err := s.logService.SaveContainerLogs(ctx, server.ID, server.Name, server.ContainerID, time.Now()); err != nil {
			logger.Error().Str("server_id", server.ID).Err(err).Msg("Failed to save container logs")
			logSaveErr = err
		}
	}

	timeout := template.StopTimeout
	if timeout <= 0 {
		timeout = 30
	}
	_, err = s.docker.ContainerStop(ctx, server.ContainerID, client.ContainerStopOptions{Timeout: &timeout})
	if err != nil {
		return err
	}

	server.Status = models.ServerStatusStopped
	if err := s.db.Save(server).Error; err != nil {
		return err
	}

	if logSaveErr != nil {
		return &ErrLogSaveFailed{Err: logSaveErr}
	}

	return nil
}

func (s *ServerService) Restart(id string) error {
	var logSaveErr *ErrLogSaveFailed
	if err := s.Stop(id); err != nil {
		// If log saving failed but server stopped successfully, continue with restart
		if errors.As(err, &logSaveErr) {
			// Continue to start, will return log save warning after
		} else {
			return err
		}
	}
	if err := s.Start(id); err != nil {
		return err
	}
	// Return log save warning if it occurred during stop
	if logSaveErr != nil {
		return logSaveErr
	}
	return nil
}

func (s *ServerService) createContainer(ctx context.Context, server *models.Server) (string, error) {
	template, _ := s.GetServerTemplate(server)

	imageExists, err := s.CheckImageExists(ctx, server.DockerImage)
	if err != nil {
		return "", fmt.Errorf("failed to check image: %w", err)
	}

	if !imageExists {
		s.mu.Lock()
		if _, exists := s.imagePullProgress[server.ID]; exists {
			s.mu.Unlock()
			return "", fmt.Errorf("image pull already in progress for this server")
		}
		progress := &ImagePullProgress{
			ServerID: server.ID,
			Image:    server.DockerImage,
			Status:   "starting",
		}
		s.imagePullProgress[server.ID] = progress
		s.mu.Unlock()

		if err := s.StartImagePull(server, progress); err != nil {
			s.mu.Lock()
			delete(s.imagePullProgress, server.ID)
			s.mu.Unlock()
			return "", fmt.Errorf("failed to pull image: %w", err)
		}

		s.mu.Lock()
		delete(s.imagePullProgress, server.ID)
		s.mu.Unlock()
	}

	env := s.templates.DecodeEnvironment(server.Environment)
	customEnv := s.templates.DecodeEnvironment(server.CustomEnvironment)
	for k, v := range customEnv {
		env[k] = v
	}
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

	exposedPorts := make(network.PortSet)
	portBindings := make(network.PortMap)
	portStr := fmt.Sprintf("%d", server.Port)
	hostIP := netip.MustParseAddr("0.0.0.0")

	protocol := template.Protocol
	if protocol == "" {
		protocol = "both"
	}

	if protocol == "tcp" || protocol == "both" {
		tcpPort := network.MustParsePort(fmt.Sprintf("%d/tcp", server.Port))
		exposedPorts[tcpPort] = struct{}{}
		portBindings[tcpPort] = []network.PortBinding{{HostIP: hostIP, HostPort: portStr}}
	}
	if protocol == "udp" || protocol == "both" {
		udpPort := network.MustParsePort(fmt.Sprintf("%d/udp", server.Port))
		exposedPorts[udpPort] = struct{}{}
		portBindings[udpPort] = []network.PortBinding{{HostIP: hostIP, HostPort: portStr}}
	}

	for _, p := range template.AdditionalPorts {
		pStr := fmt.Sprintf("%d", p.Port)
		pProtocol := strings.ToLower(p.Protocol)
		if pProtocol == "" {
			pProtocol = "both"
		}
		if pProtocol == "tcp" || pProtocol == "both" {
			port := network.MustParsePort(fmt.Sprintf("%d/tcp", p.Port))
			exposedPorts[port] = struct{}{}
			portBindings[port] = []network.PortBinding{{HostIP: hostIP, HostPort: pStr}}
		}
		if pProtocol == "udp" || pProtocol == "both" {
			port := network.MustParsePort(fmt.Sprintf("%d/udp", p.Port))
			exposedPorts[port] = struct{}{}
			portBindings[port] = []network.PortBinding{{HostIP: hostIP, HostPort: pStr}}
		}
	}

	backupPath := filepath.Join(serverPath, "../backups")
	logsPath := filepath.Join(serverPath, "../logs")

	// Translate container paths to host paths when running in Docker
	// This is necessary because Docker bind mounts require host-side paths
	serverHostPath, err := s.translatePathForDocker(ctx, serverPath)
	if err != nil {
		return "", fmt.Errorf("failed to translate server path for Docker: %w", err)
	}

	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: serverHostPath,
			Target: "/data",
		},
	}

	for _, vol := range template.Volumes {
		hostPath := ReplaceTemplateVars(vol.Host, server.ID, server.Name, serverPath, backupPath, logsPath, server.MemoryLimit, server.Port)
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(serverPath, hostPath)
		}
		hostPath = filepath.Clean(hostPath)

		// Translate path for Docker if needed
		hostMountPath, err := s.translatePathForDocker(ctx, hostPath)
		if err != nil {
			return "", fmt.Errorf("failed to translate volume path for Docker: %w", err)
		}

		readOnly := strings.ToLower(vol.Mode) == "ro"
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   hostMountPath,
			Target:   vol.Container,
			ReadOnly: readOnly,
		})
	}

	containerConfig := &container.Config{
		Image:        server.DockerImage,
		Env:          envList,
		ExposedPorts: exposedPorts,
		Tty:          true,
		OpenStdin:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	if len(template.Entrypoint) > 0 {
		containerConfig.Entrypoint = template.Entrypoint
	}
	if len(template.Cmd) > 0 {
		containerConfig.Cmd = template.Cmd
	}
	if template.User != "" {
		containerConfig.User = template.User
	}
	if template.StopSignal != "" {
		containerConfig.StopSignal = template.StopSignal
	}
	if len(template.Labels) > 0 {
		containerConfig.Labels = template.Labels
	}

	if template.HealthCheck != nil {
		containerConfig.Healthcheck = &container.HealthConfig{
			Test:          template.HealthCheck.Test,
			Interval:      template.HealthCheck.Interval,
			Timeout:       template.HealthCheck.Timeout,
			Retries:       template.HealthCheck.Retries,
			StartPeriod:   template.HealthCheck.StartPeriod,
			StartInterval: 0,
		}
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Mounts:       mounts,
		Resources: container.Resources{
			Memory: int64(server.MemoryLimit) * 1024 * 1024,
		},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}

	if len(template.CapAdd) > 0 {
		hostConfig.CapAdd = template.CapAdd
	}
	if template.NetworkMode != "" {
		hostConfig.NetworkMode = container.NetworkMode(template.NetworkMode)
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
	if err := s.db.Table("server_users").Where("server_id = ? AND user_id = ?", serverID, userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type UpdateServerRequest struct {
	Name        string
	MemoryLimit int
	Port        int
	CustomVars  map[string]string
}

func (s *ServerService) Update(id string, req UpdateServerRequest) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	template, ok := s.GetServerTemplate(server)
	if !ok {
		return fmt.Errorf("no template found for server")
	}

	proposedPort := req.Port
	if proposedPort == 0 {
		proposedPort = server.Port
	}

	proposedPorts := s.GetPortsFromTemplate(template, proposedPort)
	if err := s.ValidatePorts(proposedPorts, id); err != nil {
		return err
	}

	// Check if custom variables have actually changed
	currentCustomVars := s.templates.DecodeEnvironment(server.CustomEnvironment)
	customVarsChanged := false
	if req.CustomVars != nil {
		if len(currentCustomVars) != len(req.CustomVars) {
			customVarsChanged = true
		} else {
			for k, v := range req.CustomVars {
				cv, ok := currentCustomVars[k]
				if !ok || cv != v {
					customVarsChanged = true
					break
				}
			}
		}
	}

	isRunning := server.Status == models.ServerStatusRunning
	needsRestart := isRunning && (proposedPort != server.Port || req.MemoryLimit != server.MemoryLimit || customVarsChanged)

	if needsRestart {
		if err := s.Stop(id); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
	}

	server.Name = req.Name
	server.MemoryLimit = req.MemoryLimit
	server.Port = proposedPort

	env := s.templates.DecodeEnvironment(server.Environment)
	for k, v := range env {
		env[k] = strings.ReplaceAll(v, strconv.Itoa(server.MemoryLimit), strconv.Itoa(req.MemoryLimit))
	}
	server.Environment = s.templates.EncodeEnvironment(env)

	if req.CustomVars != nil {
		server.CustomEnvironment = s.templates.EncodeEnvironment(req.CustomVars)
	}

	if err := s.db.Save(server).Error; err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	if needsRestart {
		if err := s.Start(id); err != nil {
			return fmt.Errorf("server updated but failed to restart: %w", err)
		}
	}

	return nil
}

func (s *ServerService) ValidatePorts(ports []int, excludeServerID string) error {
	usedPorts, err := s.GetAllUsedPorts(excludeServerID)
	if err != nil {
		return err
	}
	for _, p := range ports {
		if _, ok := usedPorts[p]; ok {
			return fmt.Errorf("port %d is already in use", p)
		}
	}
	return nil
}

func (s *ServerService) HasAccess(userID, serverID string) bool {
	hasAccess, _ := s.UserHasAccess(userID, serverID)
	return hasAccess
}

func (s *ServerService) IsTemplateOutdated(server *models.Server) bool {
	template, ok := s.templates.Get(server.GameType)
	if !ok {
		return false
	}
	return server.TemplateVersion != template.Version
}

func (s *ServerService) GetTemplateVersion(gameType string) string {
	template, ok := s.templates.Get(gameType)
	if !ok {
		return ""
	}
	return template.Version
}

func (s *ServerService) UpgradeTemplate(id string) error {
	server, err := s.Get(id)
	if err != nil {
		return err
	}

	template, ok := s.templates.Get(server.GameType)
	if !ok {
		return fmt.Errorf("unknown game type: %s", server.GameType)
	}

	serverPath := filepath.Join(s.cfg.Storage.Servers, server.ID)
	if err := s.templates.ValidateTemplate(&template, serverPath); err != nil {
		return err
	}

	proposedPorts := s.GetPortsFromTemplate(template, server.Port)
	if err := s.ValidatePorts(proposedPorts, server.ID); err != nil {
		return err
	}

	if server.TemplateVersion == template.Version {
		return nil
	}

	isRunning := server.Status == models.ServerStatusRunning
	if isRunning {
		if err := s.Stop(id); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
	}

	if server.ContainerID != "" {
		ctx := context.Background()
		if _, err := s.docker.ContainerRemove(ctx, server.ContainerID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("failed to remove container: %w", err)
		}
		server.ContainerID = ""
	}

	env := make(map[string]string)
	for k, v := range template.Environment {
		v = strings.ReplaceAll(v, "{{MEMORY}}", strconv.Itoa(server.MemoryLimit))
		v = strings.ReplaceAll(v, "{{PORT}}", strconv.Itoa(server.Port))
		v = strings.ReplaceAll(v, "{{SERVER_ID}}", server.ID)
		v = strings.ReplaceAll(v, "{{SERVER_NAME}}", server.Name)
		env[k] = v
	}

	if server.CustomEnvironment != "" {
		for k, v := range s.templates.DecodeEnvironment(server.CustomEnvironment) {
			env[k] = v
		}
	}
	server.Environment = s.templates.EncodeEnvironment(env)
	server.DockerImage = template.DockerImage
	server.TemplateVersion = template.Version

	if err := s.SetServerTemplate(server, template); err != nil {
		return fmt.Errorf("failed to store template config: %w", err)
	}

	if err := s.db.Save(server).Error; err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	if isRunning {
		if err := s.Start(id); err != nil {
			return fmt.Errorf("server upgraded but failed to restart: %w", err)
		}
	}

	return nil
}

func (s *ServerService) GetPortsFromTemplate(template GameTemplate, mainPort int) []int {
	ports := []int{mainPort}
	for _, p := range template.AdditionalPorts {
		ports = append(ports, p.Port)
	}
	return ports
}

func (s *ServerService) GetAllUsedPorts(excludeServerID string) (map[int]struct{}, error) {
	used := make(map[int]struct{})
	servers, err := s.List()
	if err != nil {
		return nil, err
	}

	templateCache := make(map[string]GameTemplate)

	for _, srv := range servers {
		if srv.ID == excludeServerID {
			continue
		}

		used[srv.Port] = struct{}{}

		var template GameTemplate
		var ok bool

		if srv.TemplateConfig != "" {
			if err := json.Unmarshal([]byte(srv.TemplateConfig), &template); err == nil {
				ok = true
			}
		} else {
			if cached, found := templateCache[srv.GameType]; found {
				template = cached
				ok = true
			} else if template, ok = s.templates.Get(srv.GameType); ok {
				templateCache[srv.GameType] = template
			}
		}

		if !ok {
			continue
		}

		for _, p := range template.AdditionalPorts {
			used[p.Port] = struct{}{}
		}
	}
	return used, nil
}

package handlers

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gs-panel/internal/config"
	"gs-panel/internal/forms"
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type FileInfo struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime string
}

type FilesHandler struct {
	serverService *services.ServerService
	cfg           *config.Config
}

func NewFilesHandler(serverService *services.ServerService, cfg *config.Config) *FilesHandler {
	return &FilesHandler{serverService: serverService, cfg: cfg}
}

func (h *FilesHandler) getServerPath(serverID uint) (string, error) {
	server, err := h.serverService.Get(serverID)
	if err != nil {
		return "", err
	}
	return filepath.Join(h.cfg.Storage.Servers, server.UUID), nil
}

func (h *FilesHandler) validatePath(serverPath, relativePath string) (string, error) {
	fullPath := filepath.Join(serverPath, relativePath)
	if !strings.HasPrefix(fullPath, serverPath) {
		return "", fiber.ErrForbidden
	}
	return fullPath, nil
}

func (h *FilesHandler) List(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	server, err := h.serverService.Get(uint(serverID))
	if err != nil {
		return fiber.ErrNotFound
	}

	relativePath := filepath.Clean(c.Query("path", "/"))
	if !strings.HasPrefix(relativePath, "/") {
		relativePath = "/" + relativePath
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.UUID)
	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return fiber.ErrNotFound
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(relativePath, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("Jan 02, 2006 15:04"),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	parentPath := ""
	if relativePath != "/" {
		parentPath = filepath.Dir(relativePath)
	}

	return c.Render("servers/files", fiber.Map{
		"Title":      "Files - " + server.Name,
		"User":       middleware.GetUser(c),
		"Server":     server,
		"Files":      files,
		"Path":       relativePath,
		"ParentPath": parentPath,
	})
}

func (h *FilesHandler) View(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	server, err := h.serverService.Get(uint(serverID))
	if err != nil {
		return fiber.ErrNotFound
	}

	relativePath := c.Query("path", "")
	if relativePath == "" {
		return fiber.ErrBadRequest
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.UUID)
	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fiber.ErrNotFound
	}

	if len(content) > 1024*1024 {
		return c.Status(fiber.StatusRequestEntityTooLarge).SendString("File too large to view")
	}

	return c.Render("servers/file_view", fiber.Map{
		"Title":    filepath.Base(relativePath) + " - " + server.Name,
		"User":     middleware.GetUser(c),
		"Server":   server,
		"Path":     relativePath,
		"Content":  string(content),
		"FileName": filepath.Base(relativePath),
	})
}

func (h *FilesHandler) Save(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	serverPath, err := h.getServerPath(uint(serverID))
	if err != nil {
		return fiber.ErrNotFound
	}

	var form forms.SaveFile
	if err := c.Bind().Form(&form); err != nil {
		return fiber.ErrBadRequest
	}

	fullPath, err := h.validatePath(serverPath, form.Path)
	if err != nil {
		return err
	}

	if err := os.WriteFile(fullPath, []byte(form.Content), 0644); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) Upload(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	serverPath, err := h.getServerPath(uint(serverID))
	if err != nil {
		return fiber.ErrNotFound
	}

	var form forms.Upload
	_ = c.Bind().Form(&form)
	if form.Path == "" {
		form.Path = "/"
	}

	targetDir, err := h.validatePath(serverPath, form.Path)
	if err != nil {
		return err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return fiber.ErrBadRequest
	}

	if file.Size > config.MaxUploadSize {
		return c.Status(fiber.StatusRequestEntityTooLarge).SendString("File too large")
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(filepath.Join(targetDir, file.Filename))
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return c.Redirect().To("/servers/" + c.Params("id") + "/files?path=" + form.Path)
}

func (h *FilesHandler) Delete(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	serverPath, err := h.getServerPath(uint(serverID))
	if err != nil {
		return fiber.ErrNotFound
	}

	relativePath := c.Query("path", "")
	if relativePath == "" || relativePath == "/" {
		return fiber.ErrBadRequest
	}

	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) CreateDir(c fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	serverPath, err := h.getServerPath(uint(serverID))
	if err != nil {
		return fiber.ErrNotFound
	}

	var form forms.CreateDir
	if err := c.Bind().Form(&form); err != nil {
		return fiber.ErrBadRequest
	}

	if form.Path == "" {
		form.Path = "/"
	}
	if form.Name == "" {
		return fiber.ErrBadRequest
	}

	fullPath, err := h.validatePath(serverPath, filepath.Join(form.Path, form.Name))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	return c.Redirect().To("/servers/" + c.Params("id") + "/files?path=" + form.Path)
}

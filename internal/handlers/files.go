package handlers

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/forms"
	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/services"
	"github.com/sneakykiwi/gs-panel/views/pages/servers"

	"github.com/gofiber/fiber/v3"
)

type FilesHandler struct {
	serverService *services.ServerService
	cfg           *config.Config
}

func NewFilesHandler(serverService *services.ServerService, cfg *config.Config) *FilesHandler {
	return &FilesHandler{serverService: serverService, cfg: cfg}
}

type FileInfo struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime string
}

func (h *FilesHandler) List(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	relativePath := c.Query("path", "/")

	// Security: prevent directory traversal
	if strings.Contains(relativePath, "..") {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid path")
	}

	fullPath := filepath.Join(serverPath, relativePath)
	if !strings.HasPrefix(fullPath, serverPath) {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid path")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to read directory: "+err.Error())
	}

	var files []FileInfo
	for _, entry := range entries {
		info, _ := entry.Info()
		if info != nil {
			files = append(files, FileInfo{
				Name:    entry.Name(),
				Path:    filepath.Join(relativePath, entry.Name()),
				IsDir:   entry.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime().Format("2006-01-02 15:04"),
			})
		}
	}

	// Sort: directories first, then by name
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

	return Render(c, servers.FilesPage(middleware.GetUser(c), server, files, relativePath, parentPath))
}

func (h *FilesHandler) View(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	relativePath := c.Query("path", "")
	if relativePath == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Path is required")
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "File not found")
	}

	if len(content) > 1024*1024 {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "File too large to view (max 1MB)")
	}

	return Render(c, servers.FileViewPage(middleware.GetUser(c), server, relativePath, string(content), filepath.Base(relativePath)))
}

func (h *FilesHandler) Upload(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	var form forms.Upload
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	fullPath, err := h.validatePath(serverPath, form.Path)
	if err != nil {
		return err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "No file uploaded")
	}

	src, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to read uploaded file")
	}
	defer src.Close()

	dstPath := filepath.Join(fullPath, file.Filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create file")
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to save file")
	}

	return c.Redirect().To("/servers/" + server.ID + "/files?path=" + form.Path)
}

func (h *FilesHandler) Mkdir(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	var form forms.CreateDir
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	fullPath, err := h.validatePath(serverPath, form.Path)
	if err != nil {
		return err
	}

	newDirPath := filepath.Join(fullPath, form.Name)
	if err := os.MkdirAll(newDirPath, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create directory")
	}

	return c.Redirect().To("/servers/" + server.ID + "/files?path=" + form.Path)
}

func (h *FilesHandler) Delete(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	path := c.Query("path", "")
	if path == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Path is required")
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	fullPath, err := h.validatePath(serverPath, path)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete file")
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) Rename(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	var form forms.RenameFile
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	oldPath, err := h.validatePath(serverPath, form.OldPath)
	if err != nil {
		return err
	}

	newPath := filepath.Join(filepath.Dir(oldPath), form.NewName)
	if !strings.HasPrefix(newPath, serverPath) {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid path")
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to rename file")
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) validatePath(basePath, relativePath string) (string, error) {
	if strings.Contains(relativePath, "..") {
		return "", fiber.NewError(fiber.StatusBadRequest, "Invalid path")
	}

	fullPath := filepath.Join(basePath, relativePath)
	if !strings.HasPrefix(fullPath, basePath) {
		return "", fiber.NewError(fiber.StatusBadRequest, "Invalid path")
	}

	return fullPath, nil
}

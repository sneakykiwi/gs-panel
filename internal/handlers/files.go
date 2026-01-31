package handlers

import (
	"io"
	"os"
	"path/filepath"
	"sort"
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

func (h *FilesHandler) getServerPath(serverID string) (string, error) {
	return GetServerPathByID(h.serverService, h.cfg, serverID)
}

func (h *FilesHandler) validatePath(serverPath, relativePath string) (string, error) {
	fullPath := filepath.Join(serverPath, relativePath)
	if !strings.HasPrefix(fullPath, serverPath) {
		return "", fiber.NewError(fiber.StatusForbidden, "Access denied: path outside server directory")
	}
	return fullPath, nil
}

func (h *FilesHandler) List(c fiber.Ctx) error {
	server, err := GetServerWithAuth(c, h.serverService)
	if err != nil {
		return err
	}

	relativePath := filepath.Clean(c.Query("path", "/"))
	if !strings.HasPrefix(relativePath, "/") {
		relativePath = "/" + relativePath
	}

	serverPath := filepath.Join(h.cfg.Storage.Servers, server.ID)
	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Directory not found")
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
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	var form forms.SaveFile
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	fullPath, err := h.validatePath(serverPath, form.Path)
	if err != nil {
		return err
	}

	if err := os.WriteFile(fullPath, []byte(form.Content), 0644); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to save file: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) Upload(c fiber.Ctx) error {
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
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
		return fiber.NewError(fiber.StatusBadRequest, "No file provided")
	}

	if file.Size > config.MaxUploadSize {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "File too large (max 100MB)")
	}

	src, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer src.Close()

	dst, err := os.Create(filepath.Join(targetDir, file.Filename))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create file: "+err.Error())
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to write file: "+err.Error())
	}

	return c.Redirect().To("/servers/" + serverID + "/files?path=" + form.Path)
}

func (h *FilesHandler) Delete(c fiber.Ctx) error {
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	relativePath := c.Query("path", "")
	if relativePath == "" || relativePath == "/" {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot delete root directory")
	}

	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) CreateDir(c fiber.Ctx) error {
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	var form forms.CreateDir
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	if form.Path == "" {
		form.Path = "/"
	}
	if form.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Folder name is required")
	}

	fullPath, err := h.validatePath(serverPath, filepath.Join(form.Path, form.Name))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create folder: "+err.Error())
	}
	return c.Redirect().To("/servers/" + serverID + "/files?path=" + form.Path)
}

func (h *FilesHandler) Download(c fiber.Ctx) error {
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	relativePath := c.Query("path", "")
	if relativePath == "" || relativePath == "/" {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot download root directory")
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	fullPath, err := h.validatePath(serverPath, relativePath)
	if err != nil {
		return err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "File not found")
	}

	if info.IsDir() {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot download directories")
	}

	return c.Download(fullPath, filepath.Base(relativePath))
}

func (h *FilesHandler) Rename(c fiber.Ctx) error {
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	var form forms.RenameFile
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	if form.OldPath == "" || form.NewName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Path and new name are required")
	}

	if form.OldPath == "/" {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot rename root directory")
	}

	oldFullPath, err := h.validatePath(serverPath, form.OldPath)
	if err != nil {
		return err
	}

	newPath := filepath.Join(filepath.Dir(form.OldPath), form.NewName)
	newFullPath, err := h.validatePath(serverPath, newPath)
	if err != nil {
		return err
	}

	if err := os.Rename(oldFullPath, newFullPath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to rename: "+err.Error())
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *FilesHandler) Move(c fiber.Ctx) error {
	serverID, err := GetServerID(c)
	if err != nil {
		return err
	}

	serverPath, err := h.getServerPath(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	var form forms.MoveFile
	if err := c.Bind().Form(&form); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid form data")
	}

	if form.SourcePath == "" || form.DestPath == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Source and destination paths are required")
	}

	if form.SourcePath == "/" {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot move root directory")
	}

	sourceFullPath, err := h.validatePath(serverPath, form.SourcePath)
	if err != nil {
		return err
	}

	destDir, err := h.validatePath(serverPath, form.DestPath)
	if err != nil {
		return err
	}

	destFullPath := filepath.Join(destDir, filepath.Base(form.SourcePath))

	if err := os.Rename(sourceFullPath, destFullPath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to move: "+err.Error())
	}

	return c.SendStatus(fiber.StatusOK)
}

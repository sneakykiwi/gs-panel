package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gs-panel/internal/config"
	"gs-panel/internal/database"
	"gs-panel/internal/handlers"
	"gs-panel/internal/logger"
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/gofiber/template/html/v2"
	"github.com/moby/moby/client"
)

func main() {
	cfg := config.Load()

	if err := logger.Init(cfg.Storage.Logs); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info().Str("version", "1.0.0").Msg("Starting GS Panel")

	db, err := database.New(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to database")
	}
	logger.Info().Str("path", cfg.Database.Path).Msg("Database connected")

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Docker")
	}
	defer dockerClient.Close()
	logger.Info().Msg("Docker client connected")

	templateService := services.NewTemplateService()
	authService := services.NewAuthService(db)
	serverService := services.NewServerService(db, dockerClient, cfg, templateService)
	backupService := services.NewBackupService(db, cfg)
	schedulerService := services.NewSchedulerService(db, backupService)
	consoleService := services.NewConsoleService(dockerClient)
	statsService := services.NewStatsService(dockerClient)

	_ = consoleService
	_ = statsService

	schedulerService.LoadSchedules()
	serverService.SyncAllStatuses()

	rootDir := findRootDir()
	templatesPath := filepath.Join(rootDir, "web", "templates")
	staticPath := filepath.Join(rootDir, "web", "static")

	engine := html.New(templatesPath, ".html")
	engine.AddFunc("eq", func(a, b interface{}) bool {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	})
	engine.AddFunc("divf", func(a, b int64) float64 {
		return float64(a) / float64(b)
	})
	engine.AddFunc("dirname", func(path string) string {
		return filepath.Dir(path)
	})

	app := fiber.New(fiber.Config{
		AppName:      "GS Panel",
		Views:        engine,
		ViewsLayout:  "layouts/base",
		ErrorHandler: middleware.ErrorHandler(),
	})

	app.Use(middleware.Logger())
	app.Use("/static", static.New(staticPath))

	authHandler := handlers.NewAuthHandler(authService)
	serverHandler := handlers.NewServerHandler(serverService, templateService)
	backupHandler := handlers.NewBackupHandler(backupService, serverService, schedulerService)
	adminHandler := handlers.NewAdminHandler(authService, serverService)
	filesHandler := handlers.NewFilesHandler(serverService, cfg)

	app.Get("/login", authHandler.LoginPage)
	app.Post("/login", authHandler.Login)
	app.Get("/logout", authHandler.Logout)
	app.Get("/setup", authHandler.SetupPage)
	app.Post("/setup", authHandler.Setup)

	protected := app.Group("", middleware.Auth(authService))
	protected.Get("/", serverHandler.Dashboard)
	protected.Get("/servers/new", serverHandler.CreatePage)
	protected.Post("/servers", serverHandler.Create)
	protected.Get("/servers/:id", serverHandler.View)
	protected.Post("/servers/:id/start", serverHandler.Start)
	protected.Post("/servers/:id/stop", serverHandler.Stop)
	protected.Post("/servers/:id/restart", serverHandler.Restart)
	protected.Get("/servers/:id/status", serverHandler.Status)
	protected.Get("/servers/:id/backups", backupHandler.List)
	protected.Post("/servers/:id/backups", backupHandler.Create)
	protected.Get("/servers/:id/backups/progress", backupHandler.Progress)
	protected.Delete("/backups/:backupId", backupHandler.Delete)
	protected.Post("/backups/:backupId/restore", backupHandler.Restore)
	protected.Get("/backups/:backupId/verify", backupHandler.Verify)
	protected.Get("/servers/:id/files", filesHandler.List)
	protected.Get("/servers/:id/files/view", filesHandler.View)
	protected.Post("/servers/:id/files/save", filesHandler.Save)
	protected.Post("/servers/:id/files/upload", filesHandler.Upload)
	protected.Post("/servers/:id/files/mkdir", filesHandler.CreateDir)
	protected.Delete("/servers/:id/files", filesHandler.Delete)
	protected.Get("/servers/:id/schedules", backupHandler.ListSchedules)
	protected.Post("/servers/:id/schedules", backupHandler.CreateSchedule)
	protected.Delete("/schedules/:scheduleId", backupHandler.DeleteSchedule)
	protected.Post("/schedules/:scheduleId/toggle", backupHandler.ToggleSchedule)

	admin := protected.Group("", middleware.AdminOnly())
	admin.Delete("/servers/:id", serverHandler.Delete)
	admin.Get("/admin/users", adminHandler.UsersPage)
	admin.Get("/admin/users/new", adminHandler.CreateUserPage)
	admin.Post("/admin/users", adminHandler.CreateUser)
	admin.Delete("/admin/users/:id", adminHandler.DeleteUser)
	admin.Get("/admin/users/:id/servers", adminHandler.UserServersPage)
	admin.Post("/admin/users/:id/servers", adminHandler.AssignServer)
	admin.Delete("/admin/users/:id/servers/:serverId", adminHandler.UnassignServer)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info().Str("address", addr).Msg("Listening on")

	printBanner()

	if err := app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		logger.Fatal().Err(err).Msg("Server failed")
	}
}

func findRootDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "web")); err == nil {
			return dir
		}
	}

	wd, err := os.Getwd()
	if err == nil {
		if _, err := os.Stat(filepath.Join(wd, "web")); err == nil {
			return wd
		}
		parent := filepath.Dir(filepath.Dir(wd))
		if _, err := os.Stat(filepath.Join(parent, "web")); err == nil {
			return parent
		}
	}

	return "."
}

func printBanner() {
	banner := `
  ╔══════════════════════════════════════════════════════════════════════╗
  ║                                                                      ║
  ║    ██████╗ ███████╗    ██████╗  █████╗ ███╗   ██╗███████╗██╗         ║
  ║   ██╔════╝ ██╔════╝    ██╔══██╗██╔══██╗████╗  ██║██╔════╝██║         ║
  ║   ██║  ███╗███████╗    ██████╔╝███████║██╔██╗ ██║█████╗  ██║         ║
  ║   ██║   ██║╚════██║    ██╔═══╝ ██╔══██║██║╚██╗██║██╔══╝  ██║         ║
  ║   ╚██████╔╝███████║    ██║     ██║  ██║██║ ╚████║███████╗███████╗    ║
  ║    ╚═════╝ ╚══════╝    ╚═╝     ╚═╝  ╚═╝╚═╝  ╚═══╝╚══════╝╚══════╝    ║
  ║                                                                      ║
  ║                 Game Server Management Panel v1.0.0                  ║
  ╚══════════════════════════════════════════════════════════════════════╝
`
	fmt.Print(banner)
}

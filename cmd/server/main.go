package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/database"
	"github.com/sneakykiwi/gs-panel/internal/handlers"
	"github.com/sneakykiwi/gs-panel/internal/logger"
	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/services"
	"github.com/sneakykiwi/gs-panel/internal/version"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/moby/moby/client"
)

func main() {
	versionFlag := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version.Info())
		os.Exit(0)
	}

	cfg := config.Load()

	if err := logger.Init(cfg.Storage.Logs); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info().Str("version", version.Short()).Msg("Starting GS Panel")

	db, err := database.New(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to database")
	}
	logger.Info().Str("path", cfg.Database.Path).Msg("Database connected")

	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Docker")
	}
	defer dockerClient.Close()
	logger.Info().Msg("Docker client connected")

	templateService := services.NewTemplateService(
		cfg.Storage.DefaultTemplates,
		cfg.Storage.UserTemplates,
		&services.SecurityConfig{
			AllowedMountPrefixes: cfg.Security.AllowedMountPrefixes,
			AllowedCapAdds:       cfg.Security.AllowedCapAdds,
			EnablePerUserMounts:  cfg.Security.EnablePerUserMounts,
		},
	)
	authService := services.NewAuthService(db)
	consoleService := services.NewConsoleService(dockerClient)
	logService := services.NewLogService(cfg.Storage.Servers, dockerClient)
	serverService := services.NewServerService(db, dockerClient, cfg, templateService, consoleService, logService)
	backupService := services.NewBackupService(db, cfg)
	schedulerService := services.NewSchedulerService(db, backupService)
	statsService := services.NewStatsService(dockerClient)
	streamHandler := handlers.NewStreamHandler(serverService, consoleService, statsService)

	schedulerService.LoadSchedules()
	serverService.SyncAllStatuses()

	rootDir := findRootDir()
	staticPath := filepath.Join(rootDir, "web", "static")

	app := fiber.New(fiber.Config{
		AppName:      "GS Panel",
		ErrorHandler: middleware.ErrorHandler(),
	})

	app.Use(middleware.Logger())
	app.Use(middleware.GlobalLimiter())
	app.Use("/static", static.New(staticPath))

	//app.Use(csrf.New(csrf.Config{
	//	CookieName:     "csrf_",
	//	CookieSameSite: "Lax",
	//	CookieSecure:   false,
	//	CookieHTTPOnly: true,
	//	Extractor:      extractors.FromHeader("X-Csrf-Token"),
	//	Next: func(c fiber.Ctx) bool {
	//		path := c.Path()
	//		// Skip CSRF for login/setup pages and static assets
	//		return path == "/login" || path == "/setup" || path == "/logout" ||
	//			len(path) > 8 && path[:8] == "/static/"
	//	},
	//	ErrorHandler: func(c fiber.Ctx, err error) error {
	//		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
	//			"error": "CSRF token validation failed",
	//		})
	//	},
	//}))

	authHandler := handlers.NewAuthHandler(authService)
	serverHandler := handlers.NewServerHandler(serverService, templateService, cfg)
	backupHandler := handlers.NewBackupHandler(backupService, serverService, schedulerService, cfg)
	adminHandler := handlers.NewAdminHandler(authService, serverService)
	filesHandler := handlers.NewFilesHandler(serverService, cfg)
	logHandler := handlers.NewLogHandler(serverService, logService)
	templateHandler := handlers.NewTemplateHandler(templateService)

	app.Get("/version", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"version":    version.Short(),
			"commit":     version.GitCommit,
			"build_time": version.BuildTime,
			"go_version": version.GoVersion,
		})
	})

	app.Get("/login", authHandler.LoginPage)
	app.Post("/login", middleware.LoginLimiter(), authHandler.Login)
	app.Get("/logout", authHandler.Logout)
	app.Get("/setup", authHandler.SetupPage)
	app.Post("/setup", authHandler.Setup)

	protected := app.Group("", middleware.Auth(authService))
	protected.Get("/", serverHandler.Dashboard)
	protected.Get("/servers/new", serverHandler.CreatePage)
	protected.Get("/servers/new/advanced", serverHandler.CreateAdvancedPage)
	protected.Post("/servers/new/advanced", serverHandler.CreateAdvanced)
	protected.Post("/servers", serverHandler.Create)
	protected.Get("/servers/:id", serverHandler.View)
	protected.Get("/servers/:id/edit", serverHandler.EditPage)
	protected.Get("/servers/:id/template", serverHandler.ViewTemplateConfig)
	protected.Put("/servers/:id", serverHandler.Update)
	protected.Post("/servers/:id/start", serverHandler.Start)
	protected.Post("/servers/:id/stop", serverHandler.Stop)
	protected.Post("/servers/:id/restart", serverHandler.Restart)
	protected.Get("/servers/:id/status", serverHandler.Status)
	protected.Get("/servers/:id/status-badge", serverHandler.StatusBadge)
	protected.Get("/servers/:id/controls", serverHandler.Controls)
	protected.Get("/servers/:id/backups", backupHandler.List)
	protected.Post("/servers/:id/backups", middleware.BackupLimiter(), backupHandler.Create)
	protected.Get("/servers/:id/backups/progress", backupHandler.Progress)
	logHandler.RegisterRoutes(app)
	protected.Delete("/backups/:backupId", backupHandler.Delete)
	protected.Post("/backups/:backupId/restore", backupHandler.Restore)
	protected.Get("/backups/:backupId/verify", backupHandler.Verify)
	protected.Get("/backups/:backupId/download", backupHandler.Download)
	protected.Get("/servers/:id/files", filesHandler.List)
	protected.Get("/servers/:id/files/view", filesHandler.View)
	protected.Get("/servers/:id/files/download", filesHandler.Download)
	protected.Post("/servers/:id/files/save", filesHandler.Save)
	protected.Post("/servers/:id/files/upload", filesHandler.Upload)
	protected.Post("/servers/:id/files/mkdir", filesHandler.CreateDir)
	protected.Post("/servers/:id/files/rename", filesHandler.Rename)
	protected.Post("/servers/:id/files/move", filesHandler.Move)
	protected.Delete("/servers/:id/files", filesHandler.Delete)
	protected.Get("/servers/:id/schedules", backupHandler.ListSchedules)
	protected.Post("/servers/:id/schedules", backupHandler.CreateSchedule)
	protected.Delete("/schedules/:scheduleId", backupHandler.DeleteSchedule)
	protected.Post("/schedules/:scheduleId/toggle", backupHandler.ToggleSchedule)

	protected.Get("/servers/:id/ws", middleware.CommandLimiter(), func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			user := middleware.GetUser(c)
			c.Locals(middleware.UserContextKey, user)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}, websocket.New(streamHandler.HandleServerWS))

	admin := protected.Group("", middleware.AdminOnly())
	admin.Delete("/servers/:id", serverHandler.Delete)
	admin.Post("/servers/:id/upgrade", serverHandler.Upgrade)
	admin.Get("/admin/users", adminHandler.UsersPage)
	admin.Get("/admin/users/new", adminHandler.CreateUserPage)
	admin.Post("/admin/users", adminHandler.CreateUser)
	admin.Delete("/admin/users/:id", adminHandler.DeleteUser)
	admin.Post("/admin/users/:id/reset-password", adminHandler.ResetPassword)
	admin.Get("/admin/users/:id/servers", adminHandler.UserServersPage)
	admin.Post("/admin/users/:id/servers", adminHandler.AssignServer)
	admin.Delete("/admin/users/:id/servers/:serverId", adminHandler.UnassignServer)
	admin.Get("/admin/templates", templateHandler.ListPage)
	admin.Get("/admin/templates/new", templateHandler.CreatePage)
	admin.Post("/admin/templates/new", templateHandler.CreateFromYAML)
	admin.Post("/admin/templates/reload", templateHandler.Reload)
	admin.Get("/admin/templates/:id/edit", templateHandler.EditPage)
	admin.Post("/admin/templates/:id/edit", templateHandler.UpdateYAML)
	admin.Get("/admin/templates/:id/clone", templateHandler.ClonePage)
	admin.Post("/admin/templates/:id/clone", templateHandler.Clone)
	admin.Post("/admin/templates/:id/delete", templateHandler.DeletePage)
	admin.Get("/api/templates", templateHandler.List)
	admin.Get("/api/templates/:id", templateHandler.Get)
	admin.Post("/api/templates", templateHandler.Create)
	admin.Put("/api/templates/:id", templateHandler.Update)
	admin.Delete("/api/templates/:id", templateHandler.Delete)

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
	versionStr := version.Short()
	padding := 35 - len(versionStr)
	if padding < 0 {
		padding = 0
	}
	versionLine := fmt.Sprintf("%*s%s%*s", padding/2, "", "v"+versionStr, padding-padding/2, "")

	banner := fmt.Sprintf(`
  ╔══════════════════════════════════════════════════════════════════════╗
  ║                                                                      ║
  ║    ██████╗ ███████╗    ██████╗  █████╗ ███╗   ██╗███████╗██╗         ║
  ║   ██╔════╝ ██╔════╝    ██╔══██╗██╔══██╗████╗  ██║██╔════╝██║         ║
  ║   ██║  ███╗███████╗    ██████╔╝███████║██╔██╗ ██║█████╗  ██║         ║
  ║   ██║   ██║╚════██║    ██╔═══╝ ██╔══██║██║╚██╗██║██╔══╝  ██║         ║
  ║   ╚██████╔╝███████║    ██║     ██║  ██║██║ ╚████║███████╗███████╗    ║
  ║    ╚═════╝ ╚══════╝    ╚═╝     ╚═╝  ╚═╝╚═╝  ╚═══╝╚══════╝╚══════╝    ║
  ║                                                                      ║
  ║%s║
  ╚══════════════════════════════════════════════════════════════════════╝
`, versionLine)
	fmt.Print(banner)
}

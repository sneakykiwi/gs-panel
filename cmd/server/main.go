package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gs-panel/internal/config"
	"gs-panel/internal/database"
	"gs-panel/internal/handlers"
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/gofiber/template/html/v2"
	"github.com/moby/moby/client"
)

func main() {
	cfg := config.Load()

	db, err := database.New(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to connect to Docker: %v", err)
	}
	defer dockerClient.Close()

	templateService := services.NewTemplateService()
	authService := services.NewAuthService(db)
	serverService := services.NewServerService(db, dockerClient, cfg, templateService)

	serverService.SyncAllStatuses()

	rootDir := findRootDir()
	templatesPath := filepath.Join(rootDir, "web", "templates")
	staticPath := filepath.Join(rootDir, "web", "static")

	engine := html.New(templatesPath, ".html")
	engine.AddFunc("eq", func(a, b interface{}) bool {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	})

	app := fiber.New(fiber.Config{
		Views:       engine,
		ViewsLayout: "layouts/base",
	})

	app.Use("/static", static.New(staticPath))

	authHandler := handlers.NewAuthHandler(authService)
	serverHandler := handlers.NewServerHandler(serverService, templateService)

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

	admin := protected.Group("", middleware.AdminOnly())
	admin.Delete("/servers/:id", serverHandler.Delete)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting GS Panel on %s", addr)
	log.Fatal(app.Listen(addr))
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

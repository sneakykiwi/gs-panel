package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/models"
	"github.com/sneakykiwi/gs-panel/internal/services"

	"github.com/gofiber/contrib/v3/websocket"
)

type StreamHandler struct {
	serverService  *services.ServerService
	consoleService *services.ConsoleService
	statsService   *services.StatsService
}

func NewStreamHandler(serverService *services.ServerService, consoleService *services.ConsoleService, statsService *services.StatsService) *StreamHandler {
	return &StreamHandler{serverService: serverService, consoleService: consoleService, statsService: statsService}
}

type wsOutMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type wsInMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type statsPayload struct {
	CPU string `json:"cpu"`
	Mem string `json:"mem"`
	Rx  string `json:"rx"`
	Tx  string `json:"tx"`
}

func (h *StreamHandler) HandleServerWS(c *websocket.Conn) {
	serverID := c.Params("id")
	fmt.Printf("[WS] Attempting connection for server ID: %s\n", serverID)
	if serverID == "" {
		c.WriteJSON(wsOutMessage{Type: "error", Data: "Invalid server ID"})
		c.Close()
		return
	}

	userVal := c.Locals(middleware.UserContextKey)
	user, _ := userVal.(*models.User)
	if user == nil {
		c.WriteJSON(wsOutMessage{Type: "error", Data: "Unauthorized"})
		c.Close()
		return
	}
	if !user.IsAdmin {
		ok, err := h.serverService.UserHasAccess(user.ID, serverID)
		if err != nil {
			c.WriteJSON(wsOutMessage{Type: "error", Data: "Failed to validate access"})
			c.Close()
			return
		}
		if !ok {
			c.WriteJSON(wsOutMessage{Type: "error", Data: "Forbidden"})
			c.Close()
			return
		}
	}

	server, err := h.serverService.Get(serverID)
	if err != nil {
		fmt.Printf("[WS] Server lookup failed for ID %s: %v\n", serverID, err)
		c.WriteJSON(wsOutMessage{Type: "error", Data: "Server not found"})
		c.Close()
		return
	}
	fmt.Printf("[WS] Successfully found server: %s (ID: %s)\n", server.Name, server.ID)

	intervalMs := 1000
	if q := c.Query("interval", ""); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			intervalMs = v
		}
	}
	if intervalMs < 250 {
		intervalMs = 250
	}
	if intervalMs > 5000 {
		intervalMs = 5000
	}

	ch := h.consoleService.Subscribe(server.ID)
	defer h.consoleService.Unsubscribe(server.ID, ch)

	if server.ContainerID != "" {
		h.consoleService.StartLogStream(server.ID, server.ContainerID)
	}

	var writeMu sync.Mutex
	writeJSON := func(msg wsOutMessage) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = c.WriteJSON(msg)
	}

	writeJSON(wsOutMessage{Type: "log", Data: fmt.Sprintf("Connected to %s\n", server.Name)})
	if server.ContainerID == "" {
		writeJSON(wsOutMessage{Type: "log", Data: "Server has no container yet.\n"})
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		keepalive := time.NewTicker(10 * time.Second)
		defer keepalive.Stop()
		statsTicker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
		defer statsTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-keepalive.C:
				writeJSON(wsOutMessage{Type: "ping", Data: "pong"})
			case <-statsTicker.C:
				var payload statsPayload
				if server.ContainerID == "" {
					payload = statsPayload{CPU: "—", Mem: "—", Rx: "—", Tx: "—"}
				} else {
					updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = h.statsService.Update(updateCtx, server.ID, server.ContainerID)
					updateCancel()
					stats := h.statsService.Get(server.ID)
					if stats == nil {
						payload = statsPayload{CPU: "Error", Mem: "Error", Rx: "Error", Tx: "Error"}
					} else {
						cpu := fmt.Sprintf("%.1f%%", stats.CPUPercent)
						memUsed := math.Round(float64(stats.MemoryUsed) / 1024 / 1024)
						memLim := math.Round(float64(stats.MemoryLimit) / 1024 / 1024)
						memPct := fmt.Sprintf("%.1f", stats.MemoryPct)
						mem := fmt.Sprintf("%d / %d MB (%s%%)", int(memUsed), int(memLim), memPct)
						rx := fmt.Sprintf("%d KB", int(math.Round(float64(stats.NetRx)/1024)))
						tx := fmt.Sprintf("%d KB", int(math.Round(float64(stats.NetTx)/1024)))
						payload = statsPayload{
							CPU: cpu,
							Mem: html.EscapeString(mem),
							Rx:  html.EscapeString(rx),
							Tx:  html.EscapeString(tx),
						}
					}
				}
				writeJSON(wsOutMessage{Type: "stats", Data: payload})
			case msg, ok := <-ch:
				if !ok {
					return
				}
				writeJSON(wsOutMessage{Type: "log", Data: msg + "\n"})
			}
		}
	}()

	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			break
		}
		var in wsInMessage
		if err := json.Unmarshal(data, &in); err != nil {
			continue
		}
		if in.Type != "command" {
			continue
		}
		cmd := strings.TrimSpace(in.Data)
		if cmd == "" {
			continue
		}
		h.consoleService.Broadcast(server.ID, "> "+cmd+"\n")
		if server.ContainerID == "" {
			writeJSON(wsOutMessage{Type: "log", Data: "[error] Server container not created. Start the server first.\n"})
			continue
		}
		cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := h.consoleService.SendCommand(cmdCtx, server.ID, server.ContainerID, cmd); err != nil {
			h.consoleService.Broadcast(server.ID, "[error] "+err.Error()+"\n")
		}
		cmdCancel()
	}

	wg.Wait()
}

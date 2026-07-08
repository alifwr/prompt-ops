package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"aether-daemon/db"
	"aether-daemon/docker"
	"aether-daemon/mcp"
	"aether-daemon/metrics"
	"aether-daemon/pty"
	"aether-daemon/shell"
)

func main() {
	// Parse CLI flags
	stdioMode := flag.Bool("stdio", false, "Start the daemon in MCP stdio communication mode")
	flag.Parse()

	// Determine database path based on OS
	dbPath := "./daemon.db"
	if runtime.GOOS == "linux" {
		dbPath = "/var/promptops/daemon.db"
	}

	// In Stdio Mode, redirect general logs to stderr so stdout is reserved exclusively for JSON-RPC
	if *stdioMode {
		log.SetOutput(os.Stderr)
		if err := db.InitDB(dbPath); err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		_ = docker.InitDocker()

		runStdioLoop()
		return
	}

	// --- NORMAL VERIFICATION MODE ---
	log.Println("Starting PromptOps Daemon...")
	log.Printf("Initializing database at %s...", dbPath)
	if err := db.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully.")

	// Test shell execution helper
	log.Println("Testing shell execution wrapper...")
	var res *shell.CommandResult
	if runtime.GOOS == "windows" {
		res = shell.RunCommand(2*time.Second, "cmd", "/c", "echo Hello from PromptOps Daemon on Windows!")
	} else {
		res = shell.RunCommand(2*time.Second, "echo", "Hello from PromptOps Daemon!")
	}
	if res.ExitCode != 0 {
		log.Printf("Command failed: %s (Stderr: %s)", res.Error, res.Stderr)
	} else {
		log.Printf("Command output: %s", res.Stdout)
	}

	// Metrics Collection test
	log.Println("Starting metrics collection test...")
	stats, err := metrics.CollectStats()
	if err != nil {
		log.Printf("Error collecting stats: %v", err)
	} else {
		log.Printf("[Metrics] CPU: %.2f%% | RAM: %.2f%% | Disk: %.2f%%", stats.CPUUsage, stats.RAMUsage, stats.DiskUsage)
		if err := db.SaveMetrics(stats); err != nil {
			log.Printf("Failed to save metrics: %v", err)
		} else {
			log.Println("Metrics successfully saved to database.")
		}
	}

	// Docker Client SDK test
	log.Println("Initializing Docker Client SDK...")
	if err := docker.InitDocker(); err != nil {
		log.Printf("Docker initialization warning: %v (Proceeding in local mock mode)", err)
	} else {
		log.Println("Docker Client SDK initialized. Fetching containers...")
		containers, err := docker.ListContainers(true)
		if err != nil {
			log.Printf("Failed to list containers: %v", err)
		} else {
			log.Printf("Successfully retrieved %d docker containers.", len(containers))
			for i, c := range containers {
				log.Printf("  [%d] ID: %s | Image: %s | State: %s", i+1, c.ID, c.Image, c.State)
			}
		}
	}

	// DB backup test
	log.Println("Testing DB Backup...")
	backupFile, err := docker.BackupDatabase("a1b2c3d4e5f6", "postgres", "promptops")
	if err != nil {
		log.Printf("Failed to backup database: %v", err)
	} else {
		log.Printf("Database backup completed successfully. File written to: %s", backupFile)
		if runtime.GOOS == "windows" {
			_ = os.Remove(backupFile)
		}
	}

	// PTY Web TTY test
	log.Println("Testing PTY Spawning...")
	var shellCmd string
	if runtime.GOOS == "windows" {
		shellCmd = "cmd.exe"
	} else {
		shellCmd = "/bin/sh"
	}

	ps, err := pty.StartPty(shellCmd, nil, 24, 80)
	if err != nil {
		log.Printf("Failed to start PTY session: %v", err)
	} else {
		log.Println("PTY session started successfully.")
		if runtime.GOOS == "windows" {
			_, _ = ps.Write([]byte("exit\r\n"))
		} else {
			_, _ = ps.Write([]byte("exit\n"))
		}
		_ = ps.Close()
		log.Println("PTY session closed successfully.")
	}

	log.Println("PromptOps Daemon initialization test completed successfully!")

	// Start periodic metrics collector in the background
	log.Println("Starting background metrics collector...")
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stats, err := metrics.CollectStats()
			if err != nil {
				log.Printf("[Metrics Collector] Error collecting stats: %v", err)
				continue
			}
			if err := db.SaveMetrics(stats); err != nil {
				log.Printf("[Metrics Collector] Failed to save metrics: %v", err)
			} else {
				log.Printf("[Metrics Collector] Saved stats to DB: CPU %.2f%% | RAM %.2f%% | Disk %.2f%%", stats.CPUUsage, stats.RAMUsage, stats.DiskUsage)
			}
		}
	}()

	// Start WebSocket Gateway Client (blocks the main thread and handles reconnections)
	gatewayURL := os.Getenv("PROMPTOPS_GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "ws://127.0.0.1:3001/ws/daemon?token=dev-token-xyz"
	}
	log.Printf("Connecting to gateway at: %s", gatewayURL)
	client := NewGatewayClient(gatewayURL)
	client.Start()
}

func runStdioLoop() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp, err := mcp.HandleRequest(line)
		if err != nil {
			log.Printf("MCP handling error: %v", err)
			continue
		}

		fmt.Println(string(resp))
	}
}

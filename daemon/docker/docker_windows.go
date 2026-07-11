//go:build windows

package docker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"aether-daemon/shell"
)

var Cli *client.Client
var dockerAvailable = false

// InitDocker checks for running Docker daemon and sets up real client or falls back
func InitDocker() error {
	var err error
	Cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err = Cli.Ping(ctx)
		if err == nil {
			dockerAvailable = true
			log.Println("[Docker Windows] Connected to local Docker daemon.")
			return nil
		}
	}
	log.Println("[Docker Mock] Docker not available on Windows. Falling back to mocks.")
	return nil
}

func getContainerStatsMap() map[string][2]string {
	statsMap := make(map[string][2]string)
	res := shell.RunCommand(10*time.Second, "docker", "stats", "--no-stream", "--format", "{{.ID}} {{.CPUPerc}} {{.MemUsage}}")
	log.Printf("[Docker Stats] ExitCode: %d, Error: %s, Stdout: %q, Stderr: %q", res.ExitCode, res.Error, res.Stdout, res.Stderr)
	if res.ExitCode != 0 {
		return statsMap
	}

	lines := strings.Split(res.Stdout, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			id := fields[0]
			cpu := fields[1]
			mem := strings.Join(fields[2:], " ")
			log.Printf("[Docker Stats] Parsed container %s: CPU=%s, MEM=%s", id, cpu, mem)
			statsMap[id] = [2]string{cpu, mem}
		}
	}
	return statsMap
}

// ListContainers returns real container list if Docker is running, or mocks otherwise
func ListContainers(all bool) ([]ContainerInfo, error) {
	if !dockerAvailable {
		log.Println("[Docker Mock] Listing mock containers...")
		return []ContainerInfo{
			{
				ID:          "a1b2c3d4e5f6",
				Names:       []string{"/promptops-mock-db"},
				Image:       "postgres:16-alpine",
				State:       "running",
				Status:      "Up 2 hours",
				Ports:       []string{"5432/tcp"},
				CPUUsage:    "1.25%",
				MemoryUsage: "45.8 MiB / 16.0 GiB",
			},
			{
				ID:          "f6e5d4c3b2a1",
				Names:       []string{"/promptops-mock-app"},
				Image:       "node:22-alpine",
				State:       "exited",
				Status:      "Exited (0) 10 minutes ago",
				Ports:       []string{"3000/tcp"},
				CPUUsage:    "0.00%",
				MemoryUsage: "0 B / 0 B",
			},
		}, nil
	}

	ctx := context.Background()
	containers, err := Cli.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, err
	}

	statsMap := getContainerStatsMap()
	var list []ContainerInfo
	for _, c := range containers {
		var ports []string
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
			} else {
				ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
			}
		}

		cpu := "0.00%"
		mem := "0 B / 0 B"
		shortID := c.ID[:12]
		if vals, ok := statsMap[shortID]; ok {
			cpu = vals[0]
			mem = vals[1]
		}

		list = append(list, ContainerInfo{
			ID:          shortID,
			Names:       c.Names,
			Image:       c.Image,
			State:       c.State,
			Status:      c.Status,
			Ports:       ports,
			CPUUsage:    cpu,
			MemoryUsage: mem,
		})
	}
	return list, nil
}

// ControlContainer handles container control actions
func ControlContainer(id string, action string) error {
	if !dockerAvailable {
		log.Printf("[Docker Mock] Control container %s: %s", id, action)
		return nil
	}
	ctx := context.Background()
	var err error
	switch action {
	case "start":
		err = Cli.ContainerStart(ctx, id, container.StartOptions{})
	case "stop":
		stopTimeout := 10
		err = Cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &stopTimeout})
	case "restart":
		err = Cli.ContainerRestart(ctx, id, container.StopOptions{})
	case "remove":
		err = Cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
	return err
}

// DeployCompose writes Compose config and runs docker compose
func DeployCompose(projectName string, composeYaml string) (string, error) {
	appDir := filepath.Join("./var/promptops", "apps", projectName)

	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create app directory: %w", err)
	}

	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeYaml), 0600); err != nil {
		return "", fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

	if !dockerAvailable {
		log.Printf("[Docker Mock] Deploying compose for project %s on Windows...", projectName)
		res := shell.RunCommand(5*time.Second, "cmd", "/c", "echo Deploying Mock Compose stack completed.")
		if res.ExitCode != 0 {
			return res.Stderr, errors.New(res.Error)
		}
		return res.Stdout, nil
	}

	log.Printf("[Docker Windows] Deploying real compose project %s on Windows...", projectName)
	res := shell.RunCommand(30*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	if res.ExitCode != 0 {
		return res.Stderr, errors.New(res.Error)
	}
	return res.Stdout, nil
}

// BackupDatabase triggers postgres backup
func BackupDatabase(containerID string, username string, dbName string) (string, error) {
	backupDir := "./var/promptops/backups"
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s_%d.sql", dbName, containerID[:6], time.Now().Unix())
	backupPath := filepath.Join(backupDir, filename)

	if !dockerAvailable {
		mockSQL := fmt.Sprintf("-- Mock Database Backup\n-- Container: %s\n-- DB: %s\n-- Created: %s\nSELECT 1;\n", containerID, dbName, time.Now().Format(time.RFC3339))
		if err := os.WriteFile(backupPath, []byte(mockSQL), 0600); err != nil {
			return "", fmt.Errorf("failed to write mock sql: %w", err)
		}
		log.Printf("[Docker Mock] Database backup completed at %s", backupPath)
		return backupPath, nil
	}

	res := shell.RunCommand(10*time.Second, "docker", "exec", "-t", containerID, "pg_dump", "-U", username, dbName)
	if res.ExitCode != 0 {
		return "", fmt.Errorf("backup failed: %s", res.Stderr)
	}

	if err := os.WriteFile(backupPath, []byte(res.Stdout), 0600); err != nil {
		return "", fmt.Errorf("failed to write sql backup file: %w", err)
	}
	log.Printf("[Docker Windows] Real database backup completed at %s", backupPath)
	return backupPath, nil
}

// EnsureComposeFile writes the docker-compose.yml to disk without running docker compose up.
// Used to ensure the file exists before running control actions (stop, restart, logs).
func EnsureComposeFile(projectName string, composeYaml string) error {
	appDir := filepath.Join("./var/promptops", "apps", projectName)

	if err := os.MkdirAll(appDir, 0700); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeYaml), 0600); err != nil {
		return fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

	log.Printf("[Docker Windows] Compose file written for project %s at %s", projectName, composePath)
	return nil
}

// ControlCompose controls a docker compose project (start, stop, restart, logs, down)
func ControlCompose(projectName string, action string) (string, error) {
	appDir := filepath.Join("./var/promptops", "apps", projectName)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	if !dockerAvailable {
		log.Printf("[Docker Mock] Control compose %s: %s", projectName, action)
		return fmt.Sprintf("Mock action %s succeeded for project %s.", action, projectName), nil
	}

	var res *shell.CommandResult
	if action == "logs" {
		res = shell.RunCommand(15*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, "logs", "--tail=100")
	} else if action == "down" {
		res = shell.RunCommand(30*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, "down")
	} else {
		res = shell.RunCommand(30*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, action)
	}

	if res.ExitCode != 0 {
		return res.Stderr, errors.New(res.Error)
	}
	return res.Stdout, nil
}

// ConfigureDomain sets up a domain with Caddy for the specified project.
func ConfigureDomain(domain, email, projectName string) (string, error) {
	// Mock implementation for Windows testing
	caddyDir := filepath.Join(os.Getenv("USERPROFILE"), ".promptops", "caddy")
	if err := os.MkdirAll(caddyDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create caddy config directory: %w", err)
	}

	caddyfileContent := fmt.Sprintf(`
%s {
	tls %s
	reverse_proxy localhost:8080
}
`, domain, email)

	caddyfilePath := filepath.Join(caddyDir, "Caddyfile")
	
	f, err := os.OpenFile(caddyfilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(caddyfileContent); err != nil {
		return "", err
	}
	
	return "Domain configured successfully with Caddy (Mock Windows).", nil
}

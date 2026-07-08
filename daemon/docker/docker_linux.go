//go:build linux

package docker

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"os/exec"
	"aether-daemon/shell"
)

var Cli *client.Client

// InitDocker initializes the official Docker client SDK connection
func InitDocker() error {
	var err error
	Cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	return err
}

func getContainerStatsMap() map[string][2]string {
	statsMap := make(map[string][2]string)
	res := shell.RunCommand(10*time.Second, "docker", "stats", "--no-stream", "--format", "{{.ID}} {{.CPUPerc}} {{.MemUsage}}")
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
			statsMap[id] = [2]string{cpu, mem}
		}
	}
	return statsMap
}

// ListContainers retrieves a list of containers on the host
func ListContainers(all bool) ([]ContainerInfo, error) {
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
		// docker stats outputs 12-char long ID
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

// ControlContainer handles starts, stops, restarts, and removals
func ControlContainer(id string, action string) error {
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

// DeployCompose writes the yaml configuration and spawns docker compose CLI
func DeployCompose(projectName string, composeYaml string) (string, error) {
	appDir := filepath.Join("/var/promptops", "apps", projectName)

	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create app directory: %w", err)
	}

	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeYaml), 0600); err != nil {
		return "", fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

	log.Printf("Executing docker compose up for project: %s...", projectName)
	res := shell.RunCommand(10*time.Minute, "docker", "compose", "-p", projectName, "-f", composePath, "up", "-d", "--remove-orphans")
	if res.ExitCode != 0 {
		return res.Stderr, fmt.Errorf("docker compose failed with exit code %d: %s", res.ExitCode, res.Error)
	}

	return res.Stdout, nil
}

// BackupDatabase executes pg_dump inside a target Postgres container and redirects stdout to host
func BackupDatabase(containerID string, username string, dbName string) (string, error) {
	backupDir := "/var/promptops/backups"
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s_%d.sql", dbName, containerID[:6], time.Now().Unix())
	backupPath := filepath.Join(backupDir, filename)

	outFile, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file on host: %w", err)
	}
	defer outFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerID, "pg_dump", "-U", username, dbName)
	cmd.Stdout = outFile
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("pg_dump failed: %s (stderr: %s)", err.Error(), stderrBuf.String())
	}

	return backupPath, nil
}

// ControlCompose controls a docker compose project (start, stop, restart, logs, down)
func ControlCompose(projectName string, action string) (string, error) {
	appDir := filepath.Join("/var/promptops", "apps", projectName)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	var res *shell.CommandResult
	if action == "logs" {
		res = shell.RunCommand(15*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, "logs", "--tail=100")
	} else if action == "down" {
		res = shell.RunCommand(30*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, "down")
	} else {
		res = shell.RunCommand(30*time.Second, "docker", "compose", "-f", composePath, "-p", projectName, action)
	}

	if res.ExitCode != 0 {
		return res.Stderr, fmt.Errorf("docker compose failed with exit code %d: %s", res.ExitCode, res.Error)
	}
	return res.Stdout, nil
}

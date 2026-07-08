package mcp

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"aether-daemon/db"
	"aether-daemon/docker"
	"aether-daemon/metrics"
	"aether-daemon/shell"
)

type JsonRpcRequest struct {
	JsonRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

type JsonRpcResponse struct {
	JsonRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RpcError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type RpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type McpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type McpToolResponse struct {
	Content []McpTextContent `json:"content"`
	IsError bool             `json:"isError"`
}

type McpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// ListToolsResponse schema for MCP tools/list
type ListToolsResponse struct {
	Tools []McpTool `json:"tools"`
}

// GetAvailableTools returns the list of tools the daemon supports
func GetAvailableTools() []McpTool {
	return []McpTool{
		{
			Name:        "get_system_stats",
			Description: "Get the current CPU, RAM, and Disk utilization stats from the VPS.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "list_containers",
			Description: "List all active and stopped Docker containers with their status, image, and exposed ports.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"all": map[string]interface{}{
						"type":        "boolean",
						"description": "Show all containers, default is false (only running containers)",
					},
				},
			},
		},
		{
			Name:        "control_container",
			Description: "Start, stop, restart, or remove a Docker container by its ID or Name.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"container_id": map[string]interface{}{
						"type":        "string",
						"description": "The Docker container ID or name.",
					},
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"start", "stop", "restart", "remove"},
						"description": "The control action to trigger.",
					},
				},
				"required": []string{"container_id", "action"},
			},
		},
		{
			Name:        "deploy_compose",
			Description: "Deploy or update a Docker Compose stack given a project name and docker-compose.yml config string.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the deployment project.",
					},
					"compose_yaml": map[string]interface{}{
						"type":        "string",
						"description": "The content of the docker-compose.yml file.",
					},
				},
				"required": []string{"project_name", "compose_yaml"},
			},
		},
		{
			Name:        "backup_database",
			Description: "Back up a PostgreSQL database inside a container to a SQL backup file on the VPS host.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"container_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the running PostgreSQL container.",
					},
					"username": map[string]interface{}{
						"type":        "string",
						"description": "Postgres username.",
					},
					"database_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the target database.",
					},
				},
				"required": []string{"container_id", "username", "database_name"},
			},
		},
		{
			Name:        "run_admin_action",
			Description: "Trigger administrative actions on the VPS (reboot, docker prune, update packages, get logs, list processes).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"reboot", "docker_prune", "update_packages", "get_syslogs", "process_list"},
						"description": "The administrative action to run.",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

// HandleRequest processes an incoming JSON-RPC payload and returns the response
func HandleRequest(raw []byte) ([]byte, error) {
	var req JsonRpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorResponse(-32700, "Parse error: "+err.Error(), nil), nil
	}

	if req.JsonRPC != "2.0" {
		return errorResponse(-32600, "Invalid Request: missing jsonrpc version", req.ID), nil
	}

	var result interface{}
	var rpcErr *RpcError

	switch req.Method {
	case "tools/list":
		result = ListToolsResponse{Tools: GetAvailableTools()}

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RpcError{Code: -32602, Message: "Invalid params: " + err.Error()}
			break
		}
		result, rpcErr = handleToolCall(params.Name, params.Arguments)

	default:
		rpcErr = &RpcError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	if rpcErr != nil {
		return json.Marshal(JsonRpcResponse{
			JsonRPC: "2.0",
			Error:   rpcErr,
			ID:      req.ID,
		})
	}

	return json.Marshal(JsonRpcResponse{
		JsonRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	})
}

// HandleToolCall is the exported entry point for executing MCP tool calls from WebSocket
func HandleToolCall(name string, argsJSON []byte) (interface{}, error) {
	result, rpcErr := handleToolCall(name, json.RawMessage(argsJSON))
	if rpcErr != nil {
		return nil, fmt.Errorf("MCP error %d: %s", rpcErr.Code, rpcErr.Message)
	}
	return result, nil
}

func handleToolCall(name string, args json.RawMessage) (interface{}, *RpcError) {
	switch name {
	case "get_system_stats":
		stats, err := metrics.CollectStats()
		if err != nil {
			return nil, &RpcError{Code: -32000, Message: "Failed to collect stats: " + err.Error()}
		}
		// Save metrics to local SQLite on stats call
		_ = db.SaveMetrics(stats)

		out, _ := json.Marshal(stats)
		return McpToolResponse{
			Content: []McpTextContent{{Type: "text", Text: string(out)}},
			IsError: false,
		}, nil

	case "list_containers":
		var params struct {
			All bool `json:"all"`
		}
		_ = json.Unmarshal(args, &params)

		containers, err := docker.ListContainers(params.All)
		if err != nil {
			return McpToolResponse{
				Content: []McpTextContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			}, nil
		}

		out, _ := json.Marshal(containers)
		return McpToolResponse{
			Content: []McpTextContent{{Type: "text", Text: string(out)}},
			IsError: false,
		}, nil

	case "control_container":
		var params struct {
			ContainerID string `json:"container_id"`
			Action      string `json:"action"`
		}
		if err := json.Unmarshal(args, &params); err != nil || params.ContainerID == "" || params.Action == "" {
			return nil, &RpcError{Code: -32602, Message: "Invalid arguments"}
		}

		err := docker.ControlContainer(params.ContainerID, params.Action)
		if err != nil {
			return McpToolResponse{
				Content: []McpTextContent{{Type: "text", Text: fmt.Sprintf("Failed to control container: %s", err.Error())}},
				IsError: true,
			}, nil
		}

		return McpToolResponse{
			Content: []McpTextContent{{Type: "text", Text: fmt.Sprintf("Successfully triggered '%s' on container '%s'.", params.Action, params.ContainerID)}},
			IsError: false,
		}, nil

	case "deploy_compose":
		var params struct {
			ProjectName string `json:"project_name"`
			ComposeYaml string `json:"compose_yaml"`
		}
		if err := json.Unmarshal(args, &params); err != nil || params.ProjectName == "" || params.ComposeYaml == "" {
			return nil, &RpcError{Code: -32602, Message: "Invalid arguments"}
		}

		out, err := docker.DeployCompose(params.ProjectName, params.ComposeYaml)
		if err != nil {
			return McpToolResponse{
				Content: []McpTextContent{{Type: "text", Text: fmt.Sprintf("Deploy failed:\n%s\nError: %s", out, err.Error())}},
				IsError: true,
			}, nil
		}

		return McpToolResponse{
			Content: []McpTextContent{{Type: "text", Text: fmt.Sprintf("Deployment successfully started:\n%s", out)}},
			IsError: false,
		}, nil

	case "backup_database":
		var params struct {
			ContainerID  string `json:"container_id"`
			Username     string `json:"username"`
			DatabaseName string `json:"database_name"`
		}
		if err := json.Unmarshal(args, &params); err != nil || params.ContainerID == "" || params.Username == "" || params.DatabaseName == "" {
			return nil, &RpcError{Code: -32602, Message: "Invalid arguments"}
		}

		backupFile, err := docker.BackupDatabase(params.ContainerID, params.Username, params.DatabaseName)
		if err != nil {
			return McpToolResponse{
				Content: []McpTextContent{{Type: "text", Text: fmt.Sprintf("Database backup failed: %s", err.Error())}},
				IsError: true,
			}, nil
		}

		return McpToolResponse{
			Content: []McpTextContent{{Type: "text", Text: fmt.Sprintf("Database backup successfully written to: %s", backupFile)}},
			IsError: false,
		}, nil

	case "run_admin_action":
		var params struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(args, &params); err != nil || params.Action == "" {
			return nil, &RpcError{Code: -32602, Message: "Invalid arguments"}
		}

		var output string
		var runErr error

		switch params.Action {
		case "docker_prune":
			if runtime.GOOS == "windows" {
				output = "Mock Docker prune success. Cleared 2.45 GB of unused cache, volumes, and dangling containers."
			} else {
				res := shell.RunCommand(2*time.Minute, "docker", "system", "prune", "-af", "--volumes")
				if res.ExitCode != 0 {
					runErr = fmt.Errorf("docker prune failed: %s", res.Stderr)
				} else {
					output = res.Stdout
				}
			}

		case "reboot":
			if runtime.GOOS == "windows" {
				output = "VPS Reboot triggered (Mock)."
			} else {
				go func() {
					time.Sleep(1 * time.Second)
					shell.RunCommand(10*time.Second, "reboot")
				}()
				output = "VPS Reboot command issued successfully. Disconnecting..."
			}

		case "update_packages":
			if runtime.GOOS == "windows" {
				output = "Mock Package Update: 0 updates pending."
			} else {
				res := shell.RunCommand(5*time.Minute, "apt-get", "update", "-y")
				if res.ExitCode != 0 {
					runErr = fmt.Errorf("apt-get update failed: %s", res.Stderr)
				} else {
					output = res.Stdout
				}
			}

		case "get_syslogs":
			if runtime.GOOS == "windows" {
				output = "Mock System Logs:\n[System] Info: Windows Event logs mock output."
			} else {
				res := shell.RunCommand(10*time.Second, "tail", "-n", "50", "/var/log/syslog")
				if res.ExitCode != 0 {
					res = shell.RunCommand(10*time.Second, "journalctl", "-n", "50")
				}
				output = res.Stdout
			}

		case "process_list":
			if runtime.GOOS == "windows" {
				res := shell.RunCommand(10*time.Second, "tasklist")
				output = res.Stdout
			} else {
				res := shell.RunCommand(10*time.Second, "ps", "aux")
				output = res.Stdout
			}

		default:
			return nil, &RpcError{Code: -32602, Message: "Unknown action"}
		}

		if runErr != nil {
			return McpToolResponse{
				Content: []McpTextContent{{Type: "text", Text: runErr.Error()}},
				IsError: true,
			}, nil
		}

		return McpToolResponse{
			Content: []McpTextContent{{Type: "text", Text: output}},
			IsError: false,
		}, nil

	default:
		return nil, &RpcError{Code: -32601, Message: "Tool not found"}
	}
}

func errorResponse(code int, message string, id interface{}) []byte {
	out, _ := json.Marshal(JsonRpcResponse{
		JsonRPC: "2.0",
		Error:   &RpcError{Code: code, Message: message},
		ID:      id,
	})
	return out
}

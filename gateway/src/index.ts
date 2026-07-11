import Fastify from 'fastify';
import fs from 'fs';
import path from 'path';
import cors from '@fastify/cors';
import websocket from '@fastify/websocket';
import swagger from '@fastify/swagger';
import swaggerUi from '@fastify/swagger-ui';
import crypto from 'crypto';
import { db, pool } from './db';
import { servers, auditLogs, deployments } from './db/schema';
import { eq } from 'drizzle-orm';
import { WebSocket } from 'ws';
import { sendAgentChat } from './grpc';
import { execSync } from 'child_process';
import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import { users } from './db/schema';

const fastify = Fastify({ logger: true });

// ── In-Memory State ──
const activeDaemons = new Map<number, WebSocket>();
const ttyClientSockets = new Map<number, WebSocket>();
const pendingRpcResolvers = new Map<string, (result: any) => void>();

interface PendingApproval {
	approvalId: string;
	serverId: number;
	sessionId: string;
	toolCall: { name: string; arguments_json: string; description: string };
}
const pendingApprovals = new Map<string, PendingApproval>();

interface SessionSettings {
	alwaysApproveEnabled: boolean;
	lastActivity: number;
}
const sessionSettingsMap = new Map<string, SessionSettings>();

// ── Helper: Safety Guardrails ──
function isToolSafe(name: string, args: any): { safe: boolean; reason?: string } {
	if (name === 'execute_command') {
		const cmd = (args.command || '').toLowerCase();
		const cmdArgs = (args.args || []).map((a: any) => String(a).toLowerCase());
		const fullCmd = [cmd, ...cmdArgs].join(' ');

		const blacklist = [
			'rm -rf /',
			'rm -rf /*',
			'docker system prune',
			'mkfs',
			'dd',
			'fdisk',
		];

		for (const pattern of blacklist) {
			if (fullCmd.includes(pattern)) {
				return { safe: false, reason: `Command matches safety blacklist pattern: "${pattern}"` };
			}
		}
	}

	if (name === 'read_file' || name === 'write_file') {
		const filePath = (args.path || '').replace(/\\/g, '/').toLowerCase();

		const criticalPatterns = [
			'/etc/shadow',
			'/boot',
			'/etc/fstab',
			'id_rsa',
			'id_dsa',
			'id_ecdsa',
			'id_ed25519',
			'.ssh',
		];
		for (const pattern of criticalPatterns) {
			if (filePath.includes(pattern)) {
				return { safe: false, reason: `Access to critical system path or SSH key is blocked: "${pattern}"` };
			}
		}

		// Restrict strictly to application folders, home, var/promptops, or local dev path
		const isAllowedDir =
			filePath.includes('/home/') ||
			filePath.includes('/var/promptops/') ||
			filePath.includes('antigravity/scratch') ||
			filePath.includes('ai-devops-paas') ||
			(!filePath.startsWith('/') && !filePath.includes('..') && !filePath.includes(':'));

		if (!isAllowedDir) {
			return { safe: false, reason: `File path must be restricted to application project folders (e.g., /home/ or /var/promptops/)` };
		}
	}

	return { safe: true };
}

// ── Helper: execute a tool on a VPS Daemon ──
function executeToolOnDaemon(serverId: number, name: string, argsJson: string): Promise<any> {
	return new Promise((resolve, reject) => {
		const socket = activeDaemons.get(serverId);
		if (!socket || socket.readyState !== WebSocket.OPEN) {
			return reject(new Error('Target VPS Daemon is offline'));
		}
		const rpcId = crypto.randomUUID();
		pendingRpcResolvers.set(rpcId, resolve);
		setTimeout(() => {
			if (pendingRpcResolvers.has(rpcId)) {
				pendingRpcResolvers.delete(rpcId);
				reject(new Error('RPC request timed out on target VPS'));
			}
		}, 30000);
		socket.send(JSON.stringify({
			action: 'tools/call',
			name,
			arguments: JSON.parse(argsJson),
			rpc_id: rpcId,
		}));
	});
}

async function main() {
	// Wait for DB to be ready
	console.log('[Gateway] Verifying database connection...');
	let connected = false;
	for (let i = 0; i < 30; i++) {
		try {
			await pool.query('SELECT 1');
			connected = true;
			break;
		} catch (err) {
			console.log(`[Gateway] Database not ready yet, retrying in 1s... (${i+1}/30)`);
			await new Promise((resolve) => setTimeout(resolve, 1000));
		}
	}
	if (!connected) {
		console.error('[Gateway] Failed to connect to the database. Exiting.');
		process.exit(1);
	}
	console.log('[Gateway] Database is ready.');

	// Run drizzle-kit push automatically if in production/docker to sync schema
	if (process.env.NODE_ENV === 'production' || process.env.RUN_MIGRATIONS === 'true') {
		console.log('[Gateway] Syncing database schema with Drizzle Kit...');
		try {
			execSync('npx drizzle-kit push --force', { stdio: 'inherit' });
			console.log('[Gateway] Database schema synced successfully.');
		} catch (err: any) {
			console.error('[Gateway] Warning: Drizzle Kit push failed:', err.message);
		}
	}

	await fastify.register(cors, { origin: true });
	await fastify.register(websocket);
	await fastify.register(swagger, {
		openapi: {
			info: {
				title: 'PromptOps Gateway API',
				description: 'REST and WebSocket backend gateway for PromptOps',
				version: '1.0.0',
			},
		},
	});
	await fastify.register(swaggerUi, { routePrefix: '/documentation' });

	const JWT_SECRET = process.env.JWT_SECRET || 'promptops-super-secret';

	fastify.addHook('preHandler', async (request, reply) => {
		const publicRoutes = ['/api/auth/login', '/api/auth/register', '/health', '/db-check', '/install.sh', '/install.ps1', '/download/daemon-linux', '/download/daemon-windows', '/api/approvals', '/api/session/settings', '/api/chat'];
		if (publicRoutes.some(r => request.url.startsWith(r))) return;
		if (request.method === 'OPTIONS') return;
		if (request.url.startsWith('/api/')) {
			const authHeader = request.headers.authorization;
			console.log(`[Auth] Checking ${request.url} | Auth Header: ${authHeader ? 'PRESENT' : 'MISSING'} (${authHeader})`);
			if (!authHeader || !authHeader.startsWith('Bearer ')) {
				reply.status(401);
				return reply.send({ error: 'Unauthorized' });
			}
			try {
				const decoded = jwt.verify(authHeader.substring(7), JWT_SECRET);
				(request as any).user = decoded;
			} catch (err) {
				console.log(`[Auth] Invalid token: ${err}`);
				reply.status(401);
				return reply.send({ error: 'Invalid token' });
			}
		}
	});

	// ── REST: Auth ──
	fastify.post('/api/auth/register', async (request, reply) => {
		const { email, password } = request.body as any;
		if (!email || !password) { reply.status(400); return { error: 'Email and password required' }; }
		const hash = bcrypt.hashSync(password, 10);
		try {
			await db.insert(users).values({ email, passwordHash: hash });
			return { success: true };
		} catch (e: any) {
			reply.status(400); return { error: e.message };
		}
	});

	fastify.post('/api/auth/login', async (request, reply) => {
		const { email, password } = request.body as any;
		const user = await db.select().from(users).where(eq(users.email, email)).limit(1);
		if (user.length === 0 || !bcrypt.compareSync(password, user[0].passwordHash)) {
			reply.status(401); return { error: 'Invalid credentials' };
		}
		const token = jwt.sign({ id: user[0].id, email: user[0].email }, JWT_SECRET, { expiresIn: '7d' });
		return { token, user: { id: user[0].id, email: user[0].email } };
	});

	// ── REST: Health ──
	fastify.get('/health', async () => ({ status: 'OK', timestamp: new Date().toISOString() }));

	fastify.get('/db-check', async (_req, reply) => {
		try {
			const res = await pool.query('SELECT NOW()');
			return { status: 'Connected', time: res.rows[0].now };
		} catch (err: any) {
			reply.status(500);
			return { status: 'Disconnected', error: err.message };
		}
	});

	// ── REST: CRUD ──
	fastify.get('/api/servers', async (request) => await db.select().from(servers).where(eq(servers.userId, (request as any).user.id)));
	fastify.get('/api/audit-logs', async (request) => await db.select().from(auditLogs).where(eq(auditLogs.userId, (request as any).user.id)));

	// ── REST: Generate Registration Token ──
	fastify.post('/api/servers/generate-token', async (request, reply) => {
		const { name } = request.body as { name?: string };
		const token = crypto.randomUUID();
		const serverName = name || `vps-${token.slice(0, 8)}`;
		const gatewayHost = request.headers.host || '127.0.0.1:3001';
		const protocol = request.protocol || 'http';

		// Pre-register the server to tie it to the user
		await db.insert(servers).values({
			userId: (request as any).user.id,
			name: serverName,
			ipAddress: 'pending',
			token,
			status: 'registering'
		});

		// Build the one-liner installer commands for Linux and Windows
		const linuxCommand = `curl -sSL "${protocol}://${gatewayHost}/install.sh?token=${token}&name=${encodeURIComponent(serverName)}" | bash`;
		const windowsCommand = `irm "${protocol}://${gatewayHost}/install.ps1?token=${token}&name=${encodeURIComponent(serverName)}" | iex`;

		return {
			token,
			name: serverName,
			linux_command: linuxCommand,
			windows_command: windowsCommand,
			instructions: [
				`To register a Linux VPS, run: ${linuxCommand}`,
				`To register a Windows VPS, run: ${windowsCommand}`,
			],
		};
	});

	// ── REST: Download Daemon Binaries ──
	fastify.get('/download/daemon-linux', async (request, reply) => {
		const linuxPath = path.resolve(__dirname, '../../daemon/promptops-daemon-linux');
		if (!fs.existsSync(linuxPath)) {
			reply.status(404);
			return { error: 'Daemon binary for Linux not found. Please compile it first.' };
		}
		reply.header('Content-Type', 'application/octet-stream');
		reply.header('Content-Disposition', 'attachment; filename=promptops-daemon-linux');
		return fs.createReadStream(linuxPath);
	});

	fastify.get('/download/daemon-windows', async (request, reply) => {
		const windowsPath = path.resolve(__dirname, '../../daemon/promptops-daemon-windows.exe');
		if (!fs.existsSync(windowsPath)) {
			reply.status(404);
			return { error: 'Daemon binary for Windows not found. Please compile it first.' };
		}
		reply.header('Content-Type', 'application/octet-stream');
		reply.header('Content-Disposition', 'attachment; filename=promptops-daemon-windows.exe');
		return fs.createReadStream(windowsPath);
	});

	// ── REST: One-Liner Installer Scripts ──
	fastify.get('/install.sh', async (request, reply) => {
		const query = request.query as { token?: string; name?: string };
		const token = query.token || '';
		const name = query.name || 'promptops-vps';
		const gatewayHost = request.headers.host || '127.0.0.1:3001';
		const protocol = request.protocol || 'http';
		const wsProtocol = protocol === 'https' ? 'wss' : 'ws';

		const scriptContent = `#!/bin/bash
set -e
GATEWAY_HOST="${gatewayHost}"
TOKEN="${token}"
SERVER_NAME="${name}"
DOWNLOAD_URL="${protocol}://\${GATEWAY_HOST}/download/daemon-linux"
CONNECT_URL="${wsProtocol}://\${GATEWAY_HOST}/ws/daemon?registration=\${TOKEN}"

echo "=== PromptOps Daemon Linux Installer ==="
echo "1. Downloading daemon binary from \${DOWNLOAD_URL}..."
curl -sSL -o promptops-daemon "\${DOWNLOAD_URL}"
chmod +x promptops-daemon

echo "2. Daemon downloaded successfully."
echo "3. Starting PromptOps daemon in the background..."
nohup ./promptops-daemon --gateway "\${CONNECT_URL}" > promptops-daemon.log 2>&1 &
echo "=== Installation complete! The daemon is running in the background. ==="
`;

		reply.header('Content-Type', 'text/x-shellscript');
		return scriptContent;
	});

	fastify.get('/install.ps1', async (request, reply) => {
		const query = request.query as { token?: string; name?: string };
		const token = query.token || '';
		const name = query.name || 'promptops-vps';
		const gatewayHost = request.headers.host || '127.0.0.1:3001';
		const protocol = request.protocol || 'http';
		const wsProtocol = protocol === 'https' ? 'wss' : 'ws';

		const scriptContent = `
$GatewayHost = "${gatewayHost}"
$Token = "${token}"
$ServerName = "${name}"
$DownloadUrl = "${protocol}://$GatewayHost/download/daemon-windows"
$ConnectUrl = "${wsProtocol}://$GatewayHost/ws/daemon?registration=$Token"
$InstallDir = "$env:USERPROFILE\\promptops"

Write-Host "=== PromptOps Daemon Windows Installer ===" -ForegroundColor Cyan
Write-Host "1. Downloading daemon binary from $DownloadUrl..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Invoke-WebRequest -Uri $DownloadUrl -OutFile "$InstallDir\\promptops-daemon.exe"

Write-Host "2. Daemon downloaded successfully." -ForegroundColor Green
Write-Host "3. Starting PromptOps daemon..." -ForegroundColor Cyan
$env:PROMPTOPS_GATEWAY_URL = $ConnectUrl
Start-Process -FilePath "$InstallDir\\promptops-daemon.exe" -WindowStyle Hidden -WorkingDirectory $InstallDir
Write-Host "=== Installation complete! The daemon is running in the background. ===" -ForegroundColor Green
`;

		reply.header('Content-Type', 'text/plain');
		return scriptContent;
	});

	// ── REST: Live Metrics from Daemon ──
	fastify.get('/api/servers/:id/metrics/latest', async (request, reply) => {
		const { id } = request.params as { id: string };
		const serverId = Number(id);
		try {
			const result = await executeToolOnDaemon(serverId, 'get_system_stats', '{}');
			// result is McpToolResponse: { content: [{type, text}], isError }
			if (result && result.content && result.content.length > 0) {
				const data = JSON.parse(result.content[0].text);
				return {
					cpu: Math.round(data.cpu_usage),
					ram: data.ram_usage,
					disk: data.disk_usage,
					ramUsed: data.ram_used,
					ramTotal: data.ram_total,
					diskUsed: data.disk_used,
					diskTotal: data.disk_total
				};
			}
			return { cpu: 0, ram: 0, disk: 0, ramUsed: 0, ramTotal: 0, diskUsed: 0, diskTotal: 0 };
		} catch (err: any) {
			reply.status(503);
			return { error: 'Daemon offline or unreachable', message: err.message };
		}
	});

	// ── REST: Live Container List from Daemon ──
	fastify.get('/api/servers/:id/containers', async (request, reply) => {
		const { id } = request.params as { id: string };
		const serverId = Number(id);
		try {
			const result = await executeToolOnDaemon(serverId, 'list_containers', '{"all": true}');
			if (result && result.content && result.content.length > 0) {
				const containers = JSON.parse(result.content[0].text);
				return containers;
			}
			return [];
		} catch (err: any) {
			reply.status(503);
			return { error: 'Daemon offline or unreachable', message: err.message };
		}
	});

	// ── REST: VPS Administration Actions ──
	fastify.post('/api/servers/:id/admin-action', async (request, reply) => {
		const { id } = request.params as { id: string };
		const serverId = Number(id);
		const { action } = request.body as { action: string };
		if (!action) {
			reply.status(400);
			return { error: 'Missing action parameter' };
		}
		try {
			const result = await executeToolOnDaemon(serverId, 'run_admin_action', JSON.stringify({ action }));
			if (result && result.content && result.content.length > 0) {
				return { output: result.content[0].text, isError: result.isError };
			}
			return { output: '', isError: false };
		} catch (err: any) {
			reply.status(503);
			return { error: 'Daemon offline or unreachable', message: err.message };
		}
	});

	// ── REST: Projects (Manual Management — no AI agent) ──

	// List all projects for a server
	fastify.get('/api/servers/:id/projects', async (request, reply) => {
		const { id } = request.params as { id: string };
		const serverId = Number(id);
		const result = await db.select().from(deployments).where(eq(deployments.serverId, serverId));
		return result;
	});

	// Get single project
	fastify.get('/api/servers/:id/projects/:projectId', async (request, reply) => {
		const { projectId } = request.params as { id: string; projectId: string };
		const result = await db.select().from(deployments).where(eq(deployments.id, Number(projectId)));
		if (result.length === 0) { reply.status(404); return { error: 'Project not found' }; }
		return result[0];
	});

	// Create & deploy a new project
	fastify.post('/api/servers/:id/projects', async (request, reply) => {
		const { id } = request.params as { id: string };
		const serverId = Number(id);
		const { project_name, compose_yaml } = request.body as { project_name: string; compose_yaml: string };
		if (!project_name || !compose_yaml) { reply.status(400); return { error: 'project_name and compose_yaml are required' }; }

		try {
			await executeToolOnDaemon(serverId, 'deploy_compose', JSON.stringify({ project_name, compose_yaml }));
		} catch (err: any) {
			reply.status(503);
			return { error: 'Failed to deploy on daemon: ' + err.message };
		}

		// Persist to database
		const newDeployment = await db.insert(deployments).values({
			serverId, projectName: project_name, composeYaml: compose_yaml, status: 'running',
		}).returning();
		await db.insert(auditLogs).values({ serverId, action: 'deploy', details: `Deployed project: ${project_name}` });
		return newDeployment[0];
	});

	// Delete a project (optionally bring it down on the daemon)
	fastify.delete('/api/servers/:id/projects/:projectId', async (request, reply) => {
		const { id, projectId } = request.params as { id: string; projectId: string };
		const serverId = Number(id);
		const existing = await db.select().from(deployments).where(eq(deployments.id, Number(projectId)));
		if (existing.length === 0) { reply.status(404); return { error: 'Project not found' }; }
		const project = existing[0];

		// Best-effort: bring down on daemon
		try {
			await executeToolOnDaemon(serverId, 'control_compose_project', JSON.stringify({
				project_name: project.projectName, action: 'down'
			}));
		} catch (_) { /* daemon may be offline; proceed with DB deletion anyway */ }

		await db.delete(deployments).where(eq(deployments.id, Number(projectId)));
		await db.insert(auditLogs).values({ serverId, action: 'delete_project', details: `Deleted project: ${project.projectName}` });
		return { status: 'deleted' };
	});

	// Project action: start | stop | restart | logs
	fastify.post('/api/servers/:id/projects/:projectId/action', async (request, reply) => {
		const { id, projectId } = request.params as { id: string; projectId: string };
		const serverId = Number(id);
		const { action } = request.body as { action: 'start' | 'stop' | 'restart' | 'logs' };

		const existing = await db.select().from(deployments).where(eq(deployments.id, Number(projectId)));
		if (existing.length === 0) { reply.status(404); return { error: 'Project not found' }; }
		const project = existing[0];

		try {
			let result: any;

			if (action === 'start') {
				// deploy_compose writes the YAML to disk and runs `docker compose up -d`
				result = await executeToolOnDaemon(serverId, 'deploy_compose', JSON.stringify({
					project_name: project.projectName, compose_yaml: project.composeYaml
				}));
				await db.update(deployments).set({ status: 'running' }).where(eq(deployments.id, Number(projectId)));
				await db.insert(auditLogs).values({ serverId, action: 'project_start', details: `start: ${project.projectName}` });
				return { status: 'running', output: result?.content?.[0]?.text || 'Project started.' };

			} else if (['stop', 'restart', 'logs'].includes(action)) {
				// Pass compose_yaml so the daemon writes the file to disk before executing the action
				result = await executeToolOnDaemon(serverId, 'control_compose_project', JSON.stringify({
					project_name: project.projectName, action, compose_yaml: project.composeYaml
				}));

				if (action === 'logs') {
					if (result?.content?.[0]?.text) return { logs: result.content[0].text };
					return { logs: '' };
				}

				const newStatus = action === 'stop' ? 'stopped' : 'running';
				await db.update(deployments).set({ status: newStatus }).where(eq(deployments.id, Number(projectId)));
				await db.insert(auditLogs).values({ serverId, action: `project_${action}`, details: `${action}: ${project.projectName}` });
				return { status: newStatus, output: result?.content?.[0]?.text || '' };

			} else {
				reply.status(400);
				return { error: 'Invalid action. Must be: start, stop, restart, or logs' };
			}
		} catch (err: any) {
			reply.status(503);
			return { error: 'Daemon offline or command failed: ' + err.message };
		}
	});

	// ── REST: Session Settings ──
	fastify.post('/api/session/settings', async (request, reply) => {
		const { session_id, always_approve } = request.body as { session_id: string; always_approve: boolean };
		if (!session_id) {
			reply.status(400);
			return { error: 'Missing session_id' };
		}
		sessionSettingsMap.set(session_id, {
			alwaysApproveEnabled: always_approve,
			lastActivity: Date.now(),
		});
		return { status: 'success', always_approve };
	});

	fastify.get('/api/session/settings/:session_id', async (request, reply) => {
		const { session_id } = request.params as { session_id: string };
		const settings = sessionSettingsMap.get(session_id);
		if (!settings) return { always_approve: false };
		const active = (Date.now() - settings.lastActivity < 30 * 60 * 1000);
		return { always_approve: settings.alwaysApproveEnabled && active };
	});

	// ── REST: AI Chat (gRPC → FastAPI) ──
	fastify.post('/api/chat', async (request, reply) => {
		const { message, session_id, server_id } = request.body as {
			message: string; session_id: string; server_id: number;
		};
		if (!message || !session_id || !server_id) {
			reply.status(400);
			return { error: 'Missing required parameters' };
		}
		const daemonSocket = activeDaemons.get(server_id);
		if (!daemonSocket || daemonSocket.readyState !== WebSocket.OPEN) {
			reply.status(400);
			return { error: 'Target VPS Node is currently offline.' };
		}
		try {
			let response = await sendAgentChat({ message, session_id, server_id });

			while (response.is_approval_required && response.proposed_tool_calls.length > 0) {
				const toolCall = response.proposed_tool_calls[0];

				// 1. Safety Guardrails Validation
				const safety = isToolSafe(toolCall.name, JSON.parse(toolCall.arguments_json));
				if (!safety.safe) {
					await db.insert(auditLogs).values({
						serverId: server_id,
						action: 'blocked_tool',
						details: `Blocked tool call: ${toolCall.name} - Reason: ${safety.reason}`,
					});
					response = await sendAgentChat({
						message: `USER REJECTED: Safety guardrail blocked tool call: ${safety.reason}`,
						session_id,
						server_id,
					});
					continue;
				}

				// 2. Check if Auto-Approve is Active and Valid for this tool
				const sessionSettings = sessionSettingsMap.get(session_id);
				const alwaysApprove = sessionSettings && sessionSettings.alwaysApproveEnabled && (Date.now() - sessionSettings.lastActivity < 30 * 60 * 1000);

				if (sessionSettings) {
					sessionSettings.lastActivity = Date.now();
				}

				const isAutoApprovable = ['get_system_stats', 'list_containers', 'run_admin_action'].includes(toolCall.name);

				if (alwaysApprove && isAutoApprovable) {
					await db.insert(auditLogs).values({
						serverId: server_id,
						action: 'auto_approved_tool',
						details: `Auto-approved tool: ${toolCall.name} (${toolCall.description})`,
					});
					try {
						const result = await executeToolOnDaemon(server_id, toolCall.name, toolCall.arguments_json);
						response = await sendAgentChat({
							message: `Approved. Tool Result: ${JSON.stringify(result)}`,
							session_id,
							server_id,
						});
					} catch (execErr: any) {
						response = await sendAgentChat({
							message: `Approved. Tool Result error: ${execErr.message}`,
							session_id,
							server_id,
						});
					}
				} else {
					// Manual approval required
					const approvalId = crypto.randomUUID();
					pendingApprovals.set(approvalId, { approvalId, serverId: server_id, sessionId: session_id, toolCall });
					await db.insert(auditLogs).values({
						serverId: server_id,
						action: 'pending_approval',
						details: `Proposed tool: ${toolCall.name} (${toolCall.description})`,
					});
					return { response_text: response.response_text || 'I need your approval:', approval_required: true, approval_id: approvalId, tool_call: toolCall };
				}
			}

			return { response_text: response.response_text, approval_required: false };
		} catch (err: any) {
			fastify.log.error(err);
			reply.status(500);
			return { error: 'Agent execution failed: ' + err.message };
		}
	});

	// ── REST: Approval Response ──
	fastify.post('/api/approvals/respond', async (request, reply) => {
		const { approval_id, approve } = request.body as { approval_id: string; approve: boolean };
		if (!approval_id) { reply.status(400); return { error: 'Missing approval_id' }; }
		const pending = pendingApprovals.get(approval_id);
		if (!pending) { reply.status(404); return { error: 'Approval not found or expired' }; }
		pendingApprovals.delete(approval_id);

		if (!approve) {
			await db.insert(auditLogs).values({ serverId: pending.serverId, action: 'rejected_approval', details: `Rejected: ${pending.toolCall.name}` });
			try {
				const response = await sendAgentChat({ message: `USER REJECTED execution of ${pending.toolCall.name}.`, session_id: pending.sessionId, server_id: pending.serverId });
				return { response_text: response.response_text, status: 'rejected' };
			} catch (err: any) { reply.status(500); return { error: err.message }; }
		}

		// Safety Guardrails validation on manual approval
		const safety = isToolSafe(pending.toolCall.name, JSON.parse(pending.toolCall.arguments_json));
		if (!safety.safe) {
			await db.insert(auditLogs).values({ serverId: pending.serverId, action: 'blocked_approval', details: `Blocked: ${pending.toolCall.name} - Reason: ${safety.reason}` });
			try {
				const response = await sendAgentChat({ message: `USER REJECTED: Safety guardrail blocked tool call: ${safety.reason}`, session_id: pending.sessionId, server_id: pending.serverId });
				return { response_text: response.response_text, status: 'blocked', reason: safety.reason };
			} catch (err: any) { reply.status(500); return { error: err.message }; }
		}

		try {
			await db.insert(auditLogs).values({ serverId: pending.serverId, action: 'approved_approval', details: `Approved: ${pending.toolCall.name}` });
			const result = await executeToolOnDaemon(pending.serverId, pending.toolCall.name, pending.toolCall.arguments_json);
			const agentResponse = await sendAgentChat({ message: `Approved. Tool Result: ${JSON.stringify(result)}`, session_id: pending.sessionId, server_id: pending.serverId });
			return { response_text: agentResponse.response_text, status: 'approved' };
		} catch (err: any) { fastify.log.error(err); reply.status(500); return { error: err.message }; }
	});

	// ── WebSocket: Go Daemons ──
	// @fastify/websocket v11: first param IS the WebSocket directly
	fastify.get('/ws/daemon', { websocket: true }, (ws: any, req: any) => {
		const query = req.query as Record<string, string>;
		const token = query.token;
		const registrationToken = query.registration;
		let serverId: number | null = null;

		ws.on('message', async (message: Buffer) => {
			try {
				const payload = JSON.parse(message.toString());

				if (payload.action === 'handshake') {
					if (registrationToken) {
						const existing = await db.select().from(servers).where(eq(servers.token, registrationToken));
						if (existing.length > 0) {
							serverId = existing[0].id;
							activeDaemons.set(serverId, ws);
							await db.update(servers).set({ status: 'online', ipAddress: req.ip, name: payload.name }).where(eq(servers.id, serverId));
							ws.send(JSON.stringify({ status: 'success', server_id: serverId }));
							console.log(`[WS] Registered/Connected via registration token: ${payload.name} (ID: ${serverId})`);
						} else {
							ws.send(JSON.stringify({ status: 'error', message: 'Invalid registration token' }));
							ws.close();
							return;
						}
					} else if (token) {
						const existing = await db.select().from(servers).where(eq(servers.token, token));
						if (existing.length === 0) { ws.send(JSON.stringify({ status: 'error', message: 'Unauthorized' })); ws.close(); return; }
						serverId = existing[0].id;
						activeDaemons.set(serverId, ws);
						await db.update(servers).set({ status: 'online', ipAddress: req.ip }).where(eq(servers.id, serverId));
						ws.send(JSON.stringify({ status: 'success', server_id: serverId }));
						console.log(`[WS] Connected via token: ${existing[0].name} (ID: ${serverId})`);
					} else {
						ws.send(JSON.stringify({ status: 'error', message: 'Missing credentials' }));
						ws.close();
					}
				}

				if (payload.action === 'pty_output' && serverId !== null) {
					const clientSocket = ttyClientSockets.get(serverId);
					if (clientSocket && clientSocket.readyState === WebSocket.OPEN) {
						clientSocket.send(JSON.stringify({ type: 'stdout', data: payload.data }));
					}
				}

				if (payload.action === 'tools/response') {
					const resolve = pendingRpcResolvers.get(payload.rpc_id);
					if (resolve) { pendingRpcResolvers.delete(payload.rpc_id); resolve(payload.result); }
				}
			} catch (err: any) {
				console.error('[WS Daemon] Error:', err.message);
			}
		});

		ws.on('close', async () => {
			if (serverId !== null) {
				console.log(`[WS] Daemon offline: ID ${serverId}`);
				activeDaemons.delete(serverId);
				await db.update(servers).set({ status: 'offline' }).where(eq(servers.id, serverId));
				const clientSocket = ttyClientSockets.get(serverId);
				if (clientSocket && clientSocket.readyState === WebSocket.OPEN) {
					clientSocket.send(JSON.stringify({ type: 'stdout', data: '\r\n\x1b[31mVPS disconnected.\x1b[0m\r\n' }));
					clientSocket.close();
				}
				ttyClientSockets.delete(serverId);
			}
		});
	});

	// ── WebSocket: Frontend Clients ──
	fastify.get('/ws/client', { websocket: true }, (ws: any, req: any) => {
		const query = req.query as Record<string, string>;
		const token = query.token;
		let userId: number | null = null;
		
		if (token) {
			try {
				const decoded = jwt.verify(token, JWT_SECRET) as any;
				userId = decoded.id;
			} catch (err) {
				ws.send(JSON.stringify({ type: 'stdout', data: '\r\n\x1b[31mError: Invalid or expired authentication token.\x1b[0m\r\n' }));
				ws.close(); return;
			}
		} else {
			ws.send(JSON.stringify({ type: 'stdout', data: '\r\n\x1b[31mError: Unauthorized WebSocket connection.\x1b[0m\r\n' }));
			ws.close(); return;
		}

		let activeServerId: number | null = null;

		ws.on('message', async (message: Buffer) => {
			try {
				const payload = JSON.parse(message.toString());

				if (payload.action === 'start_tty') {
					activeServerId = payload.server_id;
					const daemonSocket = activeDaemons.get(activeServerId!);
					if (!daemonSocket || daemonSocket.readyState !== WebSocket.OPEN) {
						ws.send(JSON.stringify({ type: 'stdout', data: '\r\n\x1b[31mError: VPS offline.\x1b[0m\r\n' }));
						ws.close(); return;
					}
					ttyClientSockets.set(activeServerId!, ws);
					daemonSocket.send(JSON.stringify({ action: 'spawn_pty', cols: payload.cols || 80, rows: payload.rows || 24 }));
					console.log(`[WS] Client started TTY for server ${activeServerId}`);
					return;
				}

				if (activeServerId !== null) {
					const daemonSocket = activeDaemons.get(activeServerId);
					if (daemonSocket && daemonSocket.readyState === WebSocket.OPEN) {
						if (payload.type === 'stdin') daemonSocket.send(JSON.stringify({ action: 'pty_input', data: payload.data }));
						else if (payload.type === 'resize') daemonSocket.send(JSON.stringify({ action: 'pty_resize', cols: payload.cols, rows: payload.rows }));
					}
				}
			} catch (err: any) {
				console.error('[WS Client] Error:', err.message);
			}
		});

		ws.on('close', () => {
			if (activeServerId !== null) {
				const daemonSocket = activeDaemons.get(activeServerId);
				if (daemonSocket && daemonSocket.readyState === WebSocket.OPEN) {
					daemonSocket.send(JSON.stringify({ action: 'close_pty' }));
				}
				ttyClientSockets.delete(activeServerId);
			}
		});
	});

	const port = Number(process.env.PORT) || 3001;
	const host = process.env.HOST || '127.0.0.1';
	try {
		await fastify.listen({ port, host });
		console.log(`[Gateway] Running at http://${host}:${port}`);
	} catch (err) {
		fastify.log.error(err);
		process.exit(1);
	}
}

main();

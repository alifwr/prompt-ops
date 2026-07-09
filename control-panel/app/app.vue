<template>
  <div class="layout">
    <!-- Top Bar -->
    <header class="topbar">
      <div class="topbar-brand">
        <div class="logo">⚡</div>
        <span>PromptOps</span>
      </div>
      <div class="topbar-actions">
        <button class="btn btn-ghost btn-sm" @click="refreshMetrics">🔄 Refresh</button>
        <div class="flex items-center gap-2">
          <span class="status-dot" :class="gatewayConnected ? 'online' : 'offline'"></span>
          <span style="font-size: 12px; color: var(--text-muted)">{{ gatewayConnected ? 'Connected' : 'Offline' }}</span>
        </div>
      </div>
    </header>

    <!-- Sidebar -->
    <aside class="sidebar">
      <div
        class="nav-item"
        :class="{ active: activeTab === 'monitor' }"
        @click="activeTab = 'monitor'"
        title="Dashboard / Monitor"
      >
        📊
      </div>
      <div
        class="nav-item"
        :class="{ active: activeTab === 'projects' }"
        @click="switchToProjects"
        title="Docker Projects"
      >
        🗂
      </div>
      <div class="sidebar-divider"></div>
      
      <div
        v-for="server in servers"
        :key="server.id"
        class="server-item"
        :class="{ active: selectedServer?.id === server.id }"
        @click="selectServer(server)"
        :title="server.name + ' - ' + server.ipAddress"
      >
        <span class="status-dot" :class="server.status"></span>
        {{ server.name.substring(0, 2).toUpperCase() }}
      </div>
      
      <div class="sidebar-divider"></div>
      
      <div 
        class="server-item" 
        @click="openAddVpsModal" 
        style="border: 1.5px dashed var(--border-glass); background: transparent; font-size: 20px; color: var(--text-muted);" 
        title="Add VPS"
      >
        +
      </div>
    </aside>

    <!-- Add VPS Modal Overlay -->
    <Teleport to="body">
      <div v-if="showAddVpsModal" class="modal-overlay" @click.self="showAddVpsModal = false">
        <div class="modal-card">
          <div class="modal-header">
            <div style="display: flex; align-items: center; gap: 8px;">
              <span style="font-size: 20px;">🖥️</span>
              <span class="modal-title">Add New VPS</span>
            </div>
            <button class="btn btn-ghost btn-sm" @click="showAddVpsModal = false" style="font-size: 18px; padding: 2px 8px;">✕</button>
          </div>

          <!-- Step 1: Enter name -->
          <div v-if="!addVpsResult" style="display: flex; flex-direction: column; gap: 16px;">
            <div>
              <label class="modal-label">Server Name (optional)</label>
              <input
                v-model="addVpsName"
                class="modal-input"
                placeholder="e.g. prod-vps-02"
                @keyup.enter="generateVpsToken"
              />
            </div>
            <button class="btn btn-primary" @click="generateVpsToken" :disabled="addVpsLoading" style="width: 100%;">
              {{ addVpsLoading ? 'Generating...' : '🔑 Generate Registration Token' }}
            </button>
          </div>

          <!-- Step 2: Show command -->
          <div v-else style="display: flex; flex-direction: column; gap: 16px;">
            <div class="tabs-header" style="display: flex; gap: 8px; border-bottom: 1px solid var(--border-glass); padding-bottom: 8px;">
              <button 
                class="tab-btn" 
                :class="{ active: activeInstallTab === 'linux' }" 
                @click="activeInstallTab = 'linux'"
              >
                🐧 Linux VPS
              </button>
              <button 
                class="tab-btn" 
                :class="{ active: activeInstallTab === 'windows' }" 
                @click="activeInstallTab = 'windows'"
              >
                🪟 Windows Server
              </button>
            </div>

            <div v-if="activeInstallTab === 'linux'" style="display: flex; flex-direction: column; gap: 12px;">
              <div class="modal-step-title">Run this single command on your Linux VPS:</div>
              <div class="modal-code-block">
                <code>{{ addVpsResult.linux_command }}</code>
                <button class="btn-copy" @click="copyText(addVpsResult.linux_command)" title="Copy">📋</button>
              </div>
              <div style="font-size: 11px; color: var(--text-muted)">
                This downloads the Linux daemon from the platform, makes it executable, and runs it in the background.
              </div>
            </div>

            <div v-else style="display: flex; flex-direction: column; gap: 12px;">
              <div class="modal-step-title">Run this command in PowerShell on your Windows VPS:</div>
              <div class="modal-code-block">
                <code>{{ addVpsResult.windows_command }}</code>
                <button class="btn-copy" @click="copyText(addVpsResult.windows_command)" title="Copy">📋</button>
              </div>
              <div style="font-size: 11px; color: var(--text-muted)">
                This downloads the Windows daemon from the platform and runs it in a hidden window in the background.
              </div>
            </div>

            <div class="modal-info">
              ✅ Once the command executes, <strong>{{ addVpsResult.name }}</strong> will register and connect to the dashboard automatically.
            </div>

            <div style="display: flex; gap: 8px;">
              <button class="btn btn-ghost" @click="addVpsResult = null; activeInstallTab = 'linux'" style="flex: 1;">← Back</button>
              <button class="btn btn-primary" @click="showAddVpsModal = false; addVpsResult = null" style="flex: 1;">Close</button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Admin Action Output Modal Overlay -->
    <Teleport to="body">
      <div v-if="showOutputModal" class="modal-overlay" @click.self="showOutputModal = false">
        <div class="modal-card" style="width: 720px; max-width: 95vw;">
          <div class="modal-header">
            <div style="display: flex; align-items: center; gap: 8px;">
              <span style="font-size: 20px;">📋</span>
              <span class="modal-title">{{ outputModalTitle }}</span>
            </div>
            <button class="btn btn-ghost btn-sm" @click="showOutputModal = false" style="font-size: 18px; padding: 2px 8px;">✕</button>
          </div>
          <div style="background: #0f172a; border: 1px solid var(--border-glass); border-radius: 8px; padding: 16px; font-family: 'JetBrains Mono', monospace; font-size: 12px; color: #f8fafc; overflow-x: auto; max-height: 55vh; white-space: pre-wrap; word-break: break-all;">{{ outputModalContent }}</div>
          <div style="display: flex; gap: 8px; margin-top: 16px;">
            <button class="btn btn-ghost" @click="copyText(outputModalContent)" style="flex: 1;">📋 Copy Output</button>
            <button class="btn btn-primary" @click="showOutputModal = false" style="flex: 1;">Close</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Main Workspace -->
    <main class="workspace">


      <!-- ── TAB: MONITOR ── -->
      <template v-if="activeTab === 'monitor'">
      <!-- Metrics Row -->
      <div class="metrics-row">
        <div class="metric-card">
          <div class="metric-label">CPU Usage</div>
          <div class="metric-value">{{ metrics.cpu }}%</div>
          <div class="metric-bar">
            <div class="metric-bar-fill cpu" :style="{ width: metrics.cpu + '%' }"></div>
          </div>
          <div class="metric-subtext">Real-time CPU cores load</div>
        </div>
        <div class="metric-card">
          <div class="metric-label">Memory</div>
          <div class="metric-value">{{ metrics.ram }}%</div>
          <div class="metric-bar">
            <div class="metric-bar-fill ram" :style="{ width: metrics.ram + '%' }"></div>
          </div>
          <div class="metric-subtext" v-if="metrics.ramTotal > 0">
            {{ formatBytes(metrics.ramUsed) }} / {{ formatBytes(metrics.ramTotal) }}
          </div>
          <div class="metric-subtext" v-else>Loading stats...</div>
        </div>
        <div class="metric-card">
          <div class="metric-label">Disk</div>
          <div class="metric-value">{{ metrics.disk }}%</div>
          <div class="metric-bar">
            <div class="metric-bar-fill disk" :style="{ width: metrics.disk + '%' }"></div>
          </div>
          <div class="metric-subtext" v-if="metrics.diskTotal > 0">
            {{ formatBytes(metrics.diskUsed) }} / {{ formatBytes(metrics.diskTotal) }}
          </div>
          <div class="metric-subtext" v-else>Loading stats...</div>
        </div>
      </div>

      <!-- Server Quick Actions -->
      <div class="glass-card">
        <div class="card-header">
          <div class="card-title"><span class="icon">⚙️</span> Server Quick Actions</div>
        </div>
        <div class="quick-actions-grid">
          <button class="btn btn-ghost action-card-btn" @click="runAdminAction('reboot')" :disabled="adminActionLoading">
            <span class="action-icon">🔄</span>
            <div class="action-details">
              <div class="action-title">Reboot VPS</div>
              <div class="action-desc">Restart the host operating system</div>
            </div>
          </button>
          <button class="btn btn-ghost action-card-btn" @click="runAdminAction('docker_prune')" :disabled="adminActionLoading">
            <span class="action-icon">🧹</span>
            <div class="action-details">
              <div class="action-title">Docker Prune</div>
              <div class="action-desc">Clean unused cache, networks, and volumes</div>
            </div>
          </button>
          <button class="btn btn-ghost action-card-btn" @click="runAdminAction('update_packages')" :disabled="adminActionLoading">
            <span class="action-icon">📦</span>
            <div class="action-details">
              <div class="action-title">Update Packages</div>
              <div class="action-desc">Run system update checks (apt update)</div>
            </div>
          </button>
          <button class="btn btn-ghost action-card-btn" @click="runAdminAction('get_syslogs')" :disabled="adminActionLoading">
            <span class="action-icon">📄</span>
            <div class="action-details">
              <div class="action-title">System Logs</div>
              <div class="action-desc">View the last 50 lines of syslog</div>
            </div>
          </button>
          <button class="btn btn-ghost action-card-btn" @click="runAdminAction('process_list')" :disabled="adminActionLoading">
            <span class="action-icon">📊</span>
            <div class="action-details">
              <div class="action-title">Active Processes</div>
              <div class="action-desc">List all running CPU & memory processes</div>
            </div>
          </button>
        </div>
      </div>

      <!-- Containers -->
      <div class="glass-card">
        <div class="card-header">
          <div class="card-title"><span class="icon">📦</span> Docker Containers</div>
          <button class="btn btn-ghost btn-sm">View All</button>
        </div>
        <div>
          <div v-for="container in containers" :key="container.id" class="container-row">
            <div>
              <div class="container-name">{{ container.name }}</div>
              <div class="container-image">{{ container.image }}</div>
            </div>
            <div class="flex items-center gap-3">
              <div v-if="container.state === 'running'" class="container-metrics">
                <span class="metric-pill cpu-pill">CPU: {{ container.cpuUsage }}</span>
                <span class="metric-pill mem-pill">Mem: {{ container.memoryUsage }}</span>
              </div>
              <span class="badge" :class="'badge-' + container.state">{{ container.state }}</span>
              <button class="btn btn-ghost btn-sm btn-icon" title="Toggle">⏯</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent Activity -->
      <div class="glass-card">
        <div class="card-header">
          <div class="card-title"><span class="icon">📋</span> Recent Activity</div>
        </div>
        <div>
          <div v-for="log in auditLogs" :key="log.id" class="container-row">
            <div>
              <div style="font-size: 12px; font-weight: 500;">{{ log.action }}</div>
              <div class="container-image">{{ log.details }}</div>
            </div>
            <div class="text-muted" style="font-size: 11px;">{{ formatTime(log.createdAt) }}</div>
          </div>
          <div v-if="auditLogs.length === 0" class="text-muted" style="font-size: 13px; padding: 12px 0;">
            No activity yet. Start chatting with the AI to manage your server.
          </div>
        </div>
      </div>
      </template><!-- end monitor -->

      <!-- ── TAB: PROJECTS ── -->
      <template v-if="activeTab === 'projects'">
        <div class="glass-card" style="margin-bottom:16px;">
          <div class="card-header">
            <div class="card-title"><span class="icon">🗂</span> Docker Compose Projects</div>
            <div style="display:flex;gap:8px;">
              <button class="btn btn-ghost btn-sm" @click="selectedServer && fetchProjects(selectedServer.id)" :disabled="!selectedServer || projectsLoading">{{ projectsLoading ? '⏳' : '🔄' }} Refresh</button>
              <button class="btn btn-primary btn-sm" @click="showNewProjectModal = true" :disabled="!selectedServer">+ New Project</button>
            </div>
          </div>
          <div style="font-size:12px;color:var(--text-muted);margin-bottom:12px;">
            Manage on <strong style="color:var(--accent-primary)">{{ selectedServer?.name || 'no server selected' }}</strong> — no AI agent required.
          </div>

          <!-- Error -->
          <div v-if="projectsError" style="background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3);border-radius:8px;padding:10px 14px;font-size:12px;color:#f87171;margin-bottom:12px;">⚠️ {{ projectsError }}</div>

          <!-- Loading -->
          <div v-else-if="projectsLoading" style="text-align:center;padding:32px;color:var(--text-muted);font-size:13px;">⏳ Loading projects...</div>

          <!-- Empty -->
          <div v-else-if="projects.length === 0" style="text-align:center;padding:32px;">
            <div style="font-size:36px;margin-bottom:8px;">🗂</div>
            <div style="font-size:14px;color:var(--text-secondary);margin-bottom:4px;">No projects yet</div>
            <div style="font-size:12px;color:var(--text-muted);margin-bottom:16px;">{{ selectedServer ? 'Deploy your first Docker Compose project.' : 'Select a server first.' }}</div>
            <button v-if="selectedServer" class="btn btn-primary" @click="showNewProjectModal = true">+ Deploy First Project</button>
          </div>

          <!-- Project List -->
          <div v-else style="display:flex;flex-direction:column;gap:10px;">
            <div v-for="project in projects" :key="project.id" style="border:1px solid var(--border-glass);border-radius:10px;padding:14px;background:rgba(15,23,42,0.4);">
              <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px;">
                <div>
                  <div style="font-size:14px;font-weight:700;color:var(--text-primary);">{{ project.projectName }}</div>
                  <div style="font-size:11px;color:var(--text-muted);margin-top:2px;">Deployed {{ new Date(project.createdAt).toLocaleDateString() }}</div>
                </div>
                <span :style="project.status === 'running' ? 'background:rgba(16,185,129,0.15);color:#34d399;border:1px solid rgba(16,185,129,0.3);' : project.status === 'stopped' ? 'background:rgba(245,158,11,0.15);color:#fbbf24;border:1px solid rgba(245,158,11,0.3);' : 'background:rgba(239,68,68,0.15);color:#f87171;border:1px solid rgba(239,68,68,0.3);'" style="font-family:var(--font-mono);font-size:10px;font-weight:700;padding:2px 8px;border-radius:99px;text-transform:uppercase;">{{ project.status }}</span>
              </div>
              <div style="background:#0d1117;border:1px solid var(--border-glass);border-radius:6px;padding:8px;margin-bottom:10px;overflow:hidden;max-height:60px;">
                <pre style="font-family:var(--font-mono);font-size:10px;color:#94a3b8;white-space:pre-wrap;margin:0;overflow:hidden;">{{ project.composeYaml }}</pre>
              </div>
              <div style="display:flex;gap:6px;flex-wrap:wrap;align-items:center;">
                <button @click="triggerProjectAction(project, 'start')" :disabled="project.status === 'running' || !!projectActionLoading[project.id]" class="btn btn-success btn-sm" style="font-size:11px;">▶ Start</button>
                <button @click="triggerProjectAction(project, 'stop')" :disabled="project.status === 'stopped' || !!projectActionLoading[project.id]" class="btn btn-ghost btn-sm" style="font-size:11px;color:#fbbf24;border-color:rgba(245,158,11,0.3);">⏹ Stop</button>
                <button @click="triggerProjectAction(project, 'restart')" :disabled="!!projectActionLoading[project.id]" class="btn btn-ghost btn-sm" style="font-size:11px;">🔄 Restart</button>
                <button @click="triggerProjectAction(project, 'logs')" :disabled="!!projectActionLoading[project.id]" class="btn btn-ghost btn-sm" style="font-size:11px;">📋 Logs</button>
                <span v-if="projectActionLoading[project.id]" style="font-size:11px;color:var(--text-muted);">⏳ {{ projectActionLoading[project.id] }}...</span>
                <button @click="deleteProject(project)" :disabled="!!projectActionLoading[project.id]" class="btn btn-danger btn-sm" style="font-size:11px;margin-left:auto;">🗑 Delete</button>
              </div>
            </div>
          </div>
        </div>
      </template><!-- end projects -->

    </main>

    <!-- New Project Modal -->
    <Teleport to="body">
      <div v-if="showNewProjectModal" class="modal-overlay" @click.self="showNewProjectModal = false">
        <div class="modal-card" style="width:640px;max-width:95vw;">
          <div class="modal-header">
            <div style="display:flex;align-items:center;gap:8px;">
              <span style="font-size:20px;">🚀</span>
              <span class="modal-title">Deploy New Project</span>
            </div>
            <button class="btn btn-ghost btn-sm" @click="showNewProjectModal = false" style="font-size:18px;padding:2px 8px;">✕</button>
          </div>
          <div style="display:flex;flex-direction:column;gap:14px;">
            <div>
              <label class="modal-label">Project Name *</label>
              <input v-model="newProjectName" class="modal-input" placeholder="e.g. my-app" />
              <div style="font-size:11px;color:var(--text-muted);margin-top:4px;">Files written to: <code style="color:var(--text-secondary);">/var/promptops/{{ newProjectName || 'my-app' }}/docker-compose.yml</code></div>
            </div>
            <div>
              <label class="modal-label">docker-compose.yml *</label>
              <textarea
                v-model="newProjectYaml"
                rows="14"
                spellcheck="false"
                style="width:100%;background:#0d1117;border:1px solid var(--border-glass);border-radius:8px;padding:12px;font-family:var(--font-mono);font-size:12px;color:#f8fafc;resize:none;outline:none;line-height:1.6;box-sizing:border-box;"
                placeholder="services:&#10;  app:&#10;    image: nginx:latest&#10;    ports:&#10;      - &quot;80:80&quot;"
              ></textarea>
            </div>
          </div>
          <div style="display:flex;gap:8px;margin-top:16px;">
            <button class="btn btn-ghost" @click="showNewProjectModal = false" style="flex:1;">Cancel</button>
            <button class="btn btn-primary" @click="deployNewProject" :disabled="deployingProject || !newProjectName.trim() || !newProjectYaml.trim()" style="flex:2;">
              {{ deployingProject ? '⏳ Deploying...' : '🚀 Deploy Project' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Logs Drawer -->
    <Teleport to="body">
      <div v-if="showLogsDrawer" class="modal-overlay" @click.self="showLogsDrawer = false">
        <div class="modal-card" style="width:800px;max-width:95vw;">
          <div class="modal-header">
            <div style="display:flex;align-items:center;gap:8px;">
              <span style="font-size:20px;">📋</span>
              <span class="modal-title">Logs — {{ logsProject?.projectName }}</span>
              <span style="font-size:11px;color:var(--text-muted);">(last 100 lines)</span>
            </div>
            <div style="display:flex;gap:6px;">
              <button class="btn btn-ghost btn-sm" @click="logsProject && triggerProjectAction(logsProject, 'logs')" :disabled="logsLoading">🔄</button>
              <button class="btn btn-ghost btn-sm" @click="showLogsDrawer = false" style="font-size:18px;padding:2px 8px;">✕</button>
            </div>
          </div>
          <div style="background:#0d1117;border:1px solid var(--border-glass);border-radius:8px;padding:16px;max-height:55vh;overflow-y:auto;">
            <div v-if="logsLoading" style="text-align:center;padding:20px;color:var(--text-muted);">⏳ Loading logs...</div>
            <pre v-else style="font-family:var(--font-mono);font-size:12px;color:#f8fafc;white-space:pre-wrap;word-break:break-all;margin:0;">{{ logsContent || '(no output)' }}</pre>
          </div>
          <div style="margin-top:12px;">
            <button class="btn btn-ghost" @click="showLogsDrawer = false" style="width:100%;">Close</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- AI Chat Panel Drawer -->
    <Teleport to="body">
      <aside class="chat-drawer" :class="{ open: chatOpen }">
        <div class="chat-header">
          <div class="flex items-center gap-2">
            <span class="ai-badge">AI</span>
            <span class="chat-header-title">DevOps Assistant</span>
          </div>
          <div class="flex items-center gap-2" style="margin-left: auto;">
            <label class="toggle-switch">
              <input type="checkbox" v-model="alwaysApprove" @change="toggleAlwaysApprove" />
              <span class="slider"></span>
            </label>
            <span style="font-size: 11px; color: var(--text-secondary);" title="Bypass approvals for non-destructive commands for 30 minutes">Always Approve</span>
            <button class="btn btn-ghost btn-sm" @click="chatOpen = false" style="margin-left: 8px; font-size: 14px; padding: 2px 6px;">✕</button>
          </div>
        </div>

        <div class="chat-messages" ref="chatContainer">
          <div
            v-for="(msg, i) in chatMessages"
            :key="i"
            class="chat-bubble"
            :class="msg.role"
          >
            <div v-html="msg.content"></div>

            <!-- Approval Card -->
            <div v-if="msg.approval" class="approval-card">
              <div class="approval-card-title">⚠️ Manual Approval Required</div>
              <div class="approval-tool-name">{{ msg.approval.tool_call.name }}</div>
              <div class="approval-desc">{{ msg.approval.tool_call.description }}</div>
              <div class="approval-actions">
                <button
                  class="btn btn-success btn-sm"
                  @click="respondApproval(msg.approval.approval_id, true)"
                  :disabled="msg.approval.resolved"
                >
                  ✓ Approve
                </button>
                <button
                  class="btn btn-danger btn-sm"
                  @click="respondApproval(msg.approval.approval_id, false)"
                  :disabled="msg.approval.resolved"
                >
                  ✗ Reject
                </button>
              </div>
            </div>
          </div>

          <!-- Typing indicator -->
          <div v-if="isThinking" class="chat-bubble ai">
            <div style="display: flex; gap: 4px; padding: 4px 0;">
              <span class="typing-dot"></span>
              <span class="typing-dot" style="animation-delay: 0.15s"></span>
              <span class="typing-dot" style="animation-delay: 0.3s"></span>
            </div>
          </div>
        </div>

        <div class="chat-input-area">
          <div class="chat-input-wrapper">
            <input
              v-model="chatInput"
              class="chat-input"
              placeholder="Ask AI to manage your server..."
              @keydown.enter="sendChat"
            />
            <button class="btn btn-primary btn-sm" @click="sendChat" :disabled="!chatInput.trim() || isThinking">
              Send
            </button>
          </div>
        </div>
      </aside>

      <!-- Floating Action Button for AI -->
      <button class="ai-fab" :class="{ active: chatOpen }" @click="chatOpen = !chatOpen" title="Ask AI DevOps Assistant">
        ✨
      </button>
    </Teleport>

    <!-- Terminal Toggle -->
    <button class="terminal-toggle" @click="toggleTerminal">
      {{ terminalOpen ? '▼ Close Terminal' : '▲ Open Terminal' }}
    </button>

    <!-- Terminal Drawer -->
    <div class="terminal-drawer" :class="{ open: terminalOpen }">
      <div class="terminal-header">
        <div class="terminal-title">
          <span style="color: var(--accent-success)">●</span>
          Terminal — {{ selectedServer?.name || 'No Server' }}
        </div>
        <button class="btn btn-ghost btn-sm" @click="toggleTerminal">✕</button>
      </div>
      <div class="terminal-body" ref="terminalContainer"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick, watch } from 'vue'

const config = useRuntimeConfig()

// ── State ──
const gatewayConnected = ref(false)
const selectedServer = ref<any>(null)
const terminalOpen = ref(false)
const chatOpen = ref(false)
const chatInput = ref('')
const isThinking = ref(false)
const alwaysApprove = ref(false)
const showAddVpsModal = ref(false)
const addVpsName = ref('')
const addVpsResult = ref<any>(null)
const addVpsLoading = ref(false)
const activeInstallTab = ref('linux')
const adminActionLoading = ref(false)
const showOutputModal = ref(false)
const outputModalTitle = ref('')
const outputModalContent = ref('')

// ── Tab state ──
const activeTab = ref<'monitor' | 'projects'>('monitor')

// ── Projects state ──
const projects = ref<any[]>([])
const projectsLoading = ref(false)
const projectsError = ref('')
const showNewProjectModal = ref(false)
const newProjectName = ref('')
const newProjectYaml = ref(`services:\n  app:\n    image: nginx:latest\n    ports:\n      - "80:80"\n    restart: unless-stopped\n`)
const deployingProject = ref(false)
const showLogsDrawer = ref(false)
const logsProject = ref<any>(null)
const logsContent = ref('')
const logsLoading = ref(false)
const projectActionLoading = ref<Record<number, string>>({})

async function fetchProjects(serverId: number) {
  projectsLoading.value = true
  projectsError.value = ''
  try {
    const res = await $fetch<any[]>(`${config.public.gatewayUrl}/api/servers/${serverId}/projects`)
    projects.value = Array.isArray(res) ? res : []
  } catch (err: any) {
    projectsError.value = err.message || 'Failed to load projects'
    projects.value = []
  } finally {
    projectsLoading.value = false
  }
}

async function deployNewProject() {
  if (!selectedServer.value || !newProjectName.value.trim() || !newProjectYaml.value.trim()) return
  deployingProject.value = true
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/projects`, {
      method: 'POST',
      body: { project_name: newProjectName.value.trim(), compose_yaml: newProjectYaml.value }
    })
    projects.value.unshift(res)
    showNewProjectModal.value = false
    newProjectName.value = ''
    alert(`✅ Deployed: ${res.projectName}`)
  } catch (err: any) {
    alert(`❌ Deploy failed: ${err.message}`)
  } finally {
    deployingProject.value = false
  }
}

async function triggerProjectAction(project: any, action: 'start' | 'stop' | 'restart' | 'logs') {
  if (!selectedServer.value) return
  if (action === 'logs') {
    logsProject.value = project
    logsContent.value = ''
    showLogsDrawer.value = true
    logsLoading.value = true
    try {
      const res = await $fetch<any>(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/projects/${project.id}/action`, {
        method: 'POST', body: { action: 'logs' }
      })
      logsContent.value = res.logs || '(no output)'
    } catch (err: any) {
      logsContent.value = `Error: ${err.message}`
    } finally {
      logsLoading.value = false
    }
    return
  }
  projectActionLoading.value[project.id] = action
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/projects/${project.id}/action`, {
      method: 'POST', body: { action }
    })
    const idx = projects.value.findIndex((p: any) => p.id === project.id)
    if (idx !== -1) projects.value[idx].status = res.status || projects.value[idx].status
  } catch (err: any) {
    alert(`❌ ${action} failed: ${err.message}`)
  } finally {
    delete projectActionLoading.value[project.id]
  }
}

async function deleteProject(project: any) {
  if (!selectedServer.value) return
  if (!confirm(`Delete project "${project.projectName}"? This will also run docker compose down.`)) return
  projectActionLoading.value[project.id] = 'delete'
  try {
    await $fetch(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/projects/${project.id}`, { method: 'DELETE' })
    projects.value = projects.value.filter((p: any) => p.id !== project.id)
  } catch (err: any) {
    alert(`❌ Delete failed: ${err.message}`)
  } finally {
    delete projectActionLoading.value[project.id]
  }
}

function switchToProjects() {
  activeTab.value = 'projects'
  if (selectedServer.value) fetchProjects(selectedServer.value.id)
}

const servers = ref<any[]>([
  { id: 1, name: 'prod-vps-01', ipAddress: '100.108.255.7', status: 'online', token: 'dev-token-xyz' }
])

const metrics = ref({ 
  cpu: 42, 
  ram: 67, 
  disk: 31,
  ramUsed: 0,
  ramTotal: 0,
  diskUsed: 0,
  diskTotal: 0
})

const containers = ref([
  { id: '1', name: 'nginx-proxy', image: 'nginx:latest', state: 'running', cpuUsage: '0.12%', memoryUsage: '12.4 MiB / 16.0 GiB' },
  { id: '2', name: 'postgres-db', image: 'postgres:16', state: 'running', cpuUsage: '0.45%', memoryUsage: '45.8 MiB / 16.0 GiB' },
  { id: '3', name: 'redis-cache', image: 'redis:7-alpine', state: 'running', cpuUsage: '0.08%', memoryUsage: '8.2 MiB / 16.0 GiB' },
  { id: '4', name: 'app-backend', image: 'node:22-slim', state: 'stopped', cpuUsage: '0.00%', memoryUsage: '0 B / 0 B' },
])

const auditLogs = ref<any[]>([])
const chatMessages = ref<any[]>([
  { role: 'ai', content: 'Hello! 👋 I\'m your AI DevOps assistant. I can help you manage Docker containers, check system stats, deploy compose stacks, and backup databases. What would you like to do?' },
])

const chatContainer = ref<HTMLElement | null>(null)
const terminalContainer = ref<HTMLElement | null>(null)
const sessionId = ref('session-' + Date.now())

let term: any = null
let socket: WebSocket | null = null

// ── Methods ──
function selectServer(server: any) {
  selectedServer.value = server
}

function toggleTerminal() {
  terminalOpen.value = !terminalOpen.value
}

function formatTime(ts: string) {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleTimeString()
}

function openAddVpsModal() {
  addVpsName.value = ''
  addVpsResult.value = null
  addVpsLoading.value = false
  showAddVpsModal.value = true
}

async function generateVpsToken() {
  addVpsLoading.value = true
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/servers/generate-token`, {
      method: 'POST',
      body: { name: addVpsName.value || undefined },
    })
    addVpsResult.value = res
  } catch (err) {
    console.error('Failed to generate token', err)
  } finally {
    addVpsLoading.value = false
  }
}

function copyText(text: string) {
  navigator.clipboard.writeText(text).catch(() => {})
}

async function runAdminAction(action: string) {
  if (!selectedServer.value) return
  adminActionLoading.value = true
  
  let formattedTitle = action.replace('_', ' ').toUpperCase()
  
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/admin-action`, {
      method: 'POST',
      body: { action }
    })
    
    if (res.isError) {
      alert(`Error running action: ${res.output}`)
      return
    }

    if (action === 'get_syslogs' || action === 'process_list') {
      outputModalTitle.value = `${selectedServer.value.name} — ${formattedTitle}`
      outputModalContent.value = res.output
      showOutputModal.value = true
    } else {
      alert(`Success: ${res.output}`)
    }
  } catch (err: any) {
    console.error('Failed to run admin action', err)
    alert(`Failed to run action: ${err.message || err}`)
  } finally {
    adminActionLoading.value = false
  }
}

async function refreshMetrics() {
  if (!selectedServer.value) return
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/metrics/latest`)
    if (res && !res.error) {
      metrics.value = {
        cpu: res.cpu,
        ram: res.ram,
        disk: res.disk,
        ramUsed: res.ramUsed || 0,
        ramTotal: res.ramTotal || 0,
        diskUsed: res.diskUsed || 0,
        diskTotal: res.diskTotal || 0
      }
    }
  } catch (err) {
    console.error('Failed to fetch metrics', err)
  }
}

async function fetchContainers() {
  if (!selectedServer.value) return
  try {
    const res = await $fetch<any[]>(`${config.public.gatewayUrl}/api/servers/${selectedServer.value.id}/containers`)
    if (res && Array.isArray(res)) {
      containers.value = res.map((c: any) => ({
        id: c.id || c.ID,
        name: (c.names && c.names[0]) || c.name || c.Names?.[0] || 'unknown',
        image: c.image || c.Image || '',
        state: (c.state || c.State || 'unknown').toLowerCase(),
        cpuUsage: c.cpu_usage || c.CPUUsage || '0.00%',
        memoryUsage: c.memory_usage || c.MemoryUsage || '0 B / 0 B',
      }))
    }
  } catch (err) {
    console.error('Failed to fetch containers', err)
  }
}

async function sendChat() {
  const msg = chatInput.value.trim()
  if (!msg || isThinking.value) return

  chatMessages.value.push({ role: 'user', content: msg })
  chatInput.value = ''
  isThinking.value = true
  await scrollChat()

  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/chat`, {
      method: 'POST',
      body: {
        message: msg,
        session_id: sessionId.value,
        server_id: selectedServer.value?.id || 1,
      },
    })

    if (res.approval_required) {
      chatMessages.value.push({
        role: 'ai',
        content: res.response_text,
        approval: {
          approval_id: res.approval_id,
          tool_call: res.tool_call,
          resolved: false,
        },
      })
    } else {
      chatMessages.value.push({ role: 'ai', content: res.response_text })
    }
  } catch (err: any) {
    chatMessages.value.push({
      role: 'ai',
      content: `⚠️ Connection error: ${err.message || 'Cannot reach gateway. Is it running?'}`,
    })
  }

  isThinking.value = false
  await scrollChat()
  fetchAuditLogs()
}

async function respondApproval(approvalId: string, approve: boolean) {
  const msgIdx = chatMessages.value.findIndex((m: any) => m.approval?.approval_id === approvalId)
  if (msgIdx >= 0) {
    chatMessages.value[msgIdx].approval.resolved = true
  }

  isThinking.value = true
  chatMessages.value.push({
    role: 'user',
    content: approve ? '✅ Approved' : '❌ Rejected',
  })
  await scrollChat()

  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/approvals/respond`, {
      method: 'POST',
      body: { approval_id: approvalId, approve },
    })
    chatMessages.value.push({ role: 'ai', content: res.response_text })
  } catch (err: any) {
    chatMessages.value.push({
      role: 'ai',
      content: `⚠️ Approval response error: ${err.message}`,
    })
  }

  isThinking.value = false
  await scrollChat()
  fetchAuditLogs()
}

async function scrollChat() {
  await nextTick()
  if (chatContainer.value) {
    chatContainer.value.scrollTop = chatContainer.value.scrollHeight
  }
}

async function checkGateway() {
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/health`)
    gatewayConnected.value = res.status === 'OK'
  } catch {
    gatewayConnected.value = false
  }
}

async function fetchServers() {
  try {
    const res = await $fetch<any[]>(`${config.public.gatewayUrl}/api/servers`)
    if (res && res.length > 0) {
      servers.value = res.map(s => ({
        id: s.id,
        name: s.name,
        ipAddress: s.ipAddress,
        status: s.status,
        token: s.token
      }))
      if (!selectedServer.value) {
        selectedServer.value = servers.value[0]
      }
    }
  } catch (err) {
    console.error('Failed to fetch servers', err)
  }
}

async function fetchAuditLogs() {
  try {
    const res = await $fetch<any[]>(`${config.public.gatewayUrl}/api/audit-logs`)
    if (res) {
      auditLogs.value = res.reverse().slice(0, 5) // Show top 5 latest
    }
  } catch (err) {
    console.error('Failed to fetch audit logs', err)
  }
}

async function toggleAlwaysApprove() {
  try {
    await $fetch(`${config.public.gatewayUrl}/api/session/settings`, {
      method: 'POST',
      body: {
        session_id: sessionId.value,
        always_approve: alwaysApprove.value
      }
    })
  } catch (err) {
    console.error('Failed to sync session settings', err)
  }
}

async function checkAlwaysApproveStatus() {
  try {
    const res = await $fetch<any>(`${config.public.gatewayUrl}/api/session/settings/${sessionId.value}`)
    alwaysApprove.value = res.always_approve
  } catch (err) {
    console.error('Failed to fetch session settings', err)
  }
}

async function initTerminal() {
  if (!process.client || !terminalContainer.value) return

  if (term) {
    term.dispose()
    term = null
  }
  if (socket) {
    socket.close()
    socket = null
  }

  const { Terminal } = await import('xterm')
  const { FitAddon } = await import('xterm-addon-fit')
  
  term = new Terminal({
    cursorBlink: true,
    fontFamily: 'JetBrains Mono, Courier New, monospace',
    fontSize: 13,
    theme: {
      background: '#0d1117',
      foreground: '#c9d1d9',
      cursor: '#58a6ff',
      black: '#484f58',
      red: '#ff7b72',
      green: '#7ee787',
      yellow: '#d29922',
      blue: '#58a6ff',
      magenta: '#bc8cff',
      cyan: '#39c5cf',
      white: '#ffffff'
    }
  })

  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(terminalContainer.value)
  fitAddon.fit()

  term.write('\r\n\x1b[35m⚡ PromptOps Terminal — Connecting...\x1b[0m\r\n')

  const wsUrl = `${config.public.wsUrl.replace('http', 'ws')}/ws/client`
  socket = new WebSocket(wsUrl)

  socket.onopen = () => {
    term?.write('\x1b[32m✔ Connected to Gateway. Starting Shell...\x1b[0m\r\n')
    socket?.send(JSON.stringify({
      action: 'start_tty',
      server_id: selectedServer.value?.id || 1,
      cols: term?.cols || 80,
      rows: term?.rows || 24
    }))
  }

  socket.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data)
      if (payload.type === 'stdout' && payload.data) {
        term?.write(payload.data)
      }
    } catch {
      term?.write(event.data)
    }
  }

  socket.onclose = () => {
    term?.write('\r\n\x1b[31m❌ Connection closed.\x1b[0m\r\n')
  }

  socket.onerror = (err: any) => {
    term?.write(`\r\n\x1b[31m❌ WebSocket Error: ${err}\x1b[0m\r\n`)
  }

  term.onData((data: string) => {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({
        type: 'stdin',
        data: data
      }))
    }
  })

  window.addEventListener('resize', () => {
    try {
      fitAddon.fit()
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({
          type: 'resize',
          cols: term?.cols || 80,
          rows: term?.rows || 24
        }))
      }
    } catch (e) {
      console.warn(e)
    }
  })
}

watch(terminalOpen, (isOpen) => {
  if (isOpen) {
    nextTick(() => {
      initTerminal()
    })
  } else {
    if (socket) {
      socket.close()
      socket = null
    }
    if (term) {
      term.dispose()
      term = null
    }
  }
})

watch(selectedServer, () => {
  if (terminalOpen.value) {
    initTerminal()
  }
})

onMounted(async () => {
  await fetchServers()
  checkAlwaysApproveStatus()
  fetchAuditLogs()
  refreshMetrics()
  fetchContainers()

  checkGateway()
  setInterval(checkGateway, 10000)
  setInterval(fetchServers, 5000)
  setInterval(fetchAuditLogs, 5000)
  setInterval(refreshMetrics, 10000)
  setInterval(fetchContainers, 10000)
})

function formatBytes(bytes: number, decimals = 2) {
  if (!bytes) return '0 Bytes'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}
</script>

<style scoped>
.typing-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  animation: typingBounce 0.6s infinite alternate;
}

@keyframes typingBounce {
  from { opacity: 0.3; transform: translateY(0); }
  to { opacity: 1; transform: translateY(-4px); }
}

/* Toggle Switch Style */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 32px;
  height: 18px;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--border-glass);
  transition: .3s;
  border-radius: 18px;
}

.slider:before {
  position: absolute;
  content: "";
  height: 12px;
  width: 12px;
  left: 3px;
  bottom: 3px;
  background-color: var(--text-secondary);
  transition: .3s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: var(--accent-primary);
}

input:checked + .slider:before {
  transform: translateX(14px);
  background-color: white;
}

/* Add VPS Button */
.btn-add-vps {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  margin-top: 12px;
  padding: 10px;
  border-radius: 10px;
  border: 1.5px dashed var(--border-glass);
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}
.btn-add-vps:hover {
  border-color: var(--accent-primary);
  color: var(--accent-primary);
  background: rgba(99, 102, 241, 0.05);
}

/* Modal Overlay */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(6px);
  animation: modalFadeIn 0.2s ease;
}
@keyframes modalFadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}
.modal-card {
  width: 520px;
  max-width: 92vw;
  max-height: 85vh;
  overflow-y: auto;
  background: var(--bg-glass);
  border: 1px solid var(--border-glass);
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.4);
  animation: modalSlideIn 0.25s ease;
}
@keyframes modalSlideIn {
  from { opacity: 0; transform: translateY(16px) scale(0.97); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-glass);
}
.modal-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
}
.modal-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.modal-input {
  width: 100%;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--border-glass);
  background: var(--bg-card);
  color: var(--text-primary);
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s;
  box-sizing: border-box;
}
.modal-input:focus {
  border-color: var(--accent-primary);
}
.modal-step {
  display: flex;
  gap: 12px;
}
.modal-step-number {
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--accent-primary);
  color: white;
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 2px;
}
.modal-step-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 6px;
}
.modal-code-block {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--bg-card);
  border: 1px solid var(--border-glass);
  border-radius: 8px;
  padding: 8px 12px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  color: var(--accent-success);
  word-break: break-all;
}
.modal-code-block code {
  flex: 1;
  min-width: 0;
}
.btn-copy {
  flex-shrink: 0;
  background: none;
  border: none;
  cursor: pointer;
  font-size: 14px;
  padding: 2px;
  opacity: 0.6;
  transition: opacity 0.2s;
}
.btn-copy:hover {
  opacity: 1;
}
.modal-info {
  font-size: 13px;
  color: var(--text-secondary);
  background: rgba(16, 185, 129, 0.08);
  border: 1px solid rgba(16, 185, 129, 0.2);
  border-radius: 8px;
  padding: 10px 14px;
}
.tab-btn {
  background: transparent;
  border: none;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 600;
  padding: 8px 12px;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
  outline: none;
}
.tab-btn:hover {
  color: var(--text-primary);
}
.tab-btn.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
}

/* Quick Actions Grid */
.quick-actions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
  padding: 8px 0;
}
.action-card-btn {
  display: flex;
  align-items: center;
  gap: 12px;
  background: var(--bg-card);
  border: 1px solid var(--border-glass);
  border-radius: 12px;
  padding: 16px;
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: left;
  height: 80px;
}
.action-card-btn:hover:not(:disabled) {
  border-color: var(--accent-primary);
  background: rgba(99, 102, 241, 0.05);
  transform: translateY(-2px);
}
.action-card-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.action-icon {
  font-size: 24px;
  flex-shrink: 0;
}
.action-details {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.action-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}
.action-desc {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.metric-subtext {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 6px;
  font-weight: 500;
}
.container-metrics {
  display: flex;
  gap: 8px;
  align-items: center;
}
.metric-pill {
  font-size: 10px;
  font-family: 'JetBrains Mono', monospace;
  padding: 2px 6px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--border-glass);
}
.cpu-pill {
  color: var(--accent-success);
}
.mem-pill {
  color: var(--accent-primary);
}
</style>

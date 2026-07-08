<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'

const runtimeConfig = useRuntimeConfig()
const gatewayUrl = runtimeConfig.public.gatewayUrl as string

// Theme State
const isDark = ref(true)
const toggleTheme = () => {
  isDark.value = !isDark.value
  if (isDark.value) {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

// Servers Data — loaded from real API
const selectedServer = ref('')
const selectedServerId = ref<number | null>(null)
const servers = ref<Array<{ id: number; name: string; ipAddress: string; status: string; token: string; createdAt: string; cpu: number; ram: number }>>([]
)

const activeServerDetails = computed(() => {
  return servers.value.find(s => s.name === selectedServer.value) || servers.value[0]
})

const fetchServers = async () => {
  try {
    const res = await fetch(`${gatewayUrl}/api/servers`)
    if (!res.ok) throw new Error('Failed to fetch servers')
    const data = await res.json()
    servers.value = data.map((s: any) => ({ ...s, cpu: 0, ram: 0 }))
    if (data.length > 0 && !selectedServer.value) {
      selectedServer.value = data[0].name
      selectedServerId.value = data[0].id
      fetchContainers(data[0].id)
    }
  } catch (err) {
    console.warn('Could not load servers from API:', err)
  }
}

// Docker Containers — fetched live from daemon via gateway
const containers = ref<Array<{ id: string; name: string; image: string; ports: string; status: string; cpu_usage: string; memory_usage: string }>>([]
)
const containersLoading = ref(false)
const containersError = ref('')

const fetchContainers = async (serverId: number) => {
  containersLoading.value = true
  containersError.value = ''
  try {
    const res = await fetch(`${gatewayUrl}/api/servers/${serverId}/containers`)
    if (!res.ok) throw new Error(`Gateway returned ${res.status}`)
    const data = await res.json()
    if (Array.isArray(data)) {
      containers.value = data.map((c: any) => ({
        id: c.id || '',
        name: (c.names?.[0] || c.name || 'unknown').replace(/^\//, ''),
        image: c.image || '',
        ports: (c.ports && c.ports.length > 0) ? c.ports.join(', ') : 'none',
        status: c.state || c.status || 'unknown',
        cpu_usage: c.cpu_usage || '—',
        memory_usage: c.memory_usage || '—',
      }))
    } else if (data.error) {
      containersError.value = data.error
      containers.value = []
    }
  } catch (err: any) {
    containersError.value = err.message
    containers.value = []
  } finally {
    containersLoading.value = false
  }
}

const currentContainers = computed(() => containers.value)

// Container Actions
const toggleContainer = (index: number) => {
  const container = currentContainers.value[index]
  if (!container) return
  
  if (container.status === 'running') {
    container.status = 'stopped'
    container.uptime = '0s'
    writeTerminalOutput(`\r\n[SYSTEM] Stopping container ${container.name}...`)
    writeTerminalOutput(`\r\n[SYSTEM] Container ${container.name} stopped.`)
    addNotification(`Container ${container.name} stopped.`)
  } else {
    container.status = 'running'
    container.uptime = 'Just started'
    writeTerminalOutput(`\r\n[SYSTEM] Starting container ${container.name}...`)
    writeTerminalOutput(`\r\n[SYSTEM] Container ${container.name} started successfully (Port: ${container.ports}).`)
    addNotification(`Container ${container.name} started.`)
  }
  writeTerminalOutput('\r\n$ ')
}

const restartContainer = (name: string) => {
  const container = currentContainers.value.find(c => c.name === name)
  if (!container) return
  
  writeTerminalOutput(`\r\n$ docker restart ${name}`)
  writeTerminalOutput(`\r\nStopping container ${name}...`)
  container.status = 'stopped'
  
  setTimeout(() => {
    container.status = 'running'
    container.uptime = 'Just restarted'
    writeTerminalOutput(`\r\nStarting container ${name}...`)
    writeTerminalOutput(`\r\nContainer ${name} restarted successfully.`)
    writeTerminalOutput('\r\n$ ')
    addNotification(`Container ${name} restarted.`)
  }, 1000)
}

// Chart.js Setup
let cpuChart: any = null
let ramChart: any = null
let chartInterval: any = null

// Terminal Setup (xterm.js)
let term: any = null
let fitAddon: any = null
let socket: WebSocket | null = null
const isTerminalConnected = ref(false)
const terminalSize = ref<'min' | 'half' | 'full'>('half') // 'min' | 'half' | 'full'
let currentLine = ''

// AI Chat Console Data
const chatInput = ref('')
const chatHistory = ref([
  {
    sender: 'ai',
    text: 'Hello! I am your PromptOps AI DevOps Agent. I can help you monitor server resources, manage Docker containers, trigger system backups, and deploy configurations. Ask me a question or try typing `/backup VPS A`.',
    time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
])

// Approvals Queue
const approvals = ref([
  {
    id: 'app-1',
    title: 'Schedule System Backups',
    server: 'VPS A',
    action: 'Cron Job Creation',
    file: '/etc/cron.d/backup-vps-a',
    diff: `- # No backup job configured\n+ 0 2 * * * root /usr/local/bin/backup.sh --all >> /var/log/backup.log 2>&1`,
    metadata: {
      'Target Path': '/etc/cron.d/backup-vps-a',
      'Interval': 'Daily at 02:00 AM',
      'Backup Target': 'All Docker Volumes',
      'Retention Policy': '7 days rotation'
    },
    status: 'pending' // 'pending' | 'approved' | 'rejected'
  }
])

// Notifications Toasts
const notifications = ref<Array<{ id: number; text: string }>>([])
const addNotification = (text: string) => {
  const id = Date.now()
  notifications.value.push({ id, text })
  setTimeout(() => {
    notifications.value = notifications.value.filter(n => n.id !== id)
  }, 3500)
}

// Chart Setup inside onMounted
const initCharts = (ChartJS: any) => {
  const cpuCtx = document.getElementById('cpuChart') as HTMLCanvasElement
  const ramCtx = document.getElementById('ramChart') as HTMLCanvasElement

  if (!cpuCtx || !ramCtx) return

  const isOnline = activeServerDetails.value.status === 'online'
  const initialLabels = Array(10).fill('').map((_, i) => '')
  
  cpuChart = new ChartJS(cpuCtx, {
    type: 'line',
    data: {
      labels: initialLabels,
      datasets: [{
        label: 'CPU Usage',
        data: Array(10).fill(isOnline ? Math.floor(Math.random() * 20) + 15 : 0),
        borderColor: 'rgba(99, 102, 241, 1)', // Indigo 500
        backgroundColor: 'rgba(99, 102, 241, 0.1)',
        borderWidth: 2,
        pointRadius: 2,
        tension: 0.4,
        fill: true
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: { enabled: true }
      },
      scales: {
        y: {
          min: 0,
          max: 100,
          ticks: { color: isDark.value ? '#94a3b8' : '#64748b', font: { size: 10 } },
          grid: { color: isDark.value ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)' }
        },
        x: {
          ticks: { color: isDark.value ? '#94a3b8' : '#64748b', font: { size: 10 } },
          grid: { display: false }
        }
      }
    }
  })

  ramChart = new ChartJS(ramCtx, {
    type: 'line',
    data: {
      labels: initialLabels,
      datasets: [{
        label: 'RAM Usage',
        data: Array(10).fill(isOnline ? Math.floor(Math.random() * 15) + 40 : 0),
        borderColor: 'rgba(168, 85, 247, 1)', // Purple 500
        backgroundColor: 'rgba(168, 85, 247, 0.1)',
        borderWidth: 2,
        pointRadius: 2,
        tension: 0.4,
        fill: true
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: { enabled: true }
      },
      scales: {
        y: {
          min: 0,
          max: 100,
          ticks: { color: isDark.value ? '#94a3b8' : '#64748b', font: { size: 10 } },
          grid: { color: isDark.value ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)' }
        },
        x: {
          ticks: { color: isDark.value ? '#94a3b8' : '#64748b', font: { size: 10 } },
          grid: { display: false }
        }
      }
    }
  })
}

// Chart real-time simulation
const startChartSimulation = () => {
  chartInterval = setInterval(() => {
    const isOnline = activeServerDetails.value.status === 'online'
    const cpuVal = isOnline ? Math.floor(Math.random() * (40 - 15 + 1)) + 15 : 0
    const ramVal = isOnline ? Math.floor(Math.random() * (75 - 55 + 1)) + 55 : 0

    // Update servers health display variables
    const s = servers.value.find(serv => serv.id === selectedServer.value)
    if (s) {
      s.cpu = cpuVal
      s.ram = ramVal
    }

    if (cpuChart && cpuChart.data) {
      cpuChart.data.labels.push('')
      cpuChart.data.datasets[0].data.push(cpuVal)
      if (cpuChart.data.labels.length > 10) {
        cpuChart.data.labels.shift()
        cpuChart.data.datasets[0].data.shift()
      }
      cpuChart.update('none')
    }

    if (ramChart && ramChart.data) {
      ramChart.data.labels.push('')
      ramChart.data.datasets[0].data.push(ramVal)
      if (ramChart.data.labels.length > 10) {
        ramChart.data.labels.shift()
        ramChart.data.datasets[0].data.shift()
      }
      ramChart.update('none')
    }
  }, 2000)
}

// WebSocket Stream connection
const connectWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/ws/client`
  
  try {
    socket = new WebSocket(wsUrl)
    
    socket.onopen = () => {
      isTerminalConnected.value = true
      term?.writeln('\r\n\x1b[1;32m✓ Active connection established with remote VPS Agent stream.\x1b[0m\r\n')
      term?.write('$ ')
    }
    
    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        if (msg.event === 'output') {
          term?.write(msg.data)
        }
      } catch (err) {
        term?.write(event.data)
      }
    }
    
    socket.onerror = () => {
      fallbackToMockShell()
    }
    
    socket.onclose = () => {
      fallbackToMockShell()
    }
  } catch (e) {
    fallbackToMockShell()
  }
}

const writeTerminalOutput = (text: string) => {
  if (term) {
    term.write(text)
  }
}

const fallbackToMockShell = () => {
  if (isTerminalConnected.value) {
    isTerminalConnected.value = false
    term?.writeln('\r\n\x1b[1;31m❌ WebSocket stream disconnected.\x1b[0m\r\n')
  }
  setupMockShell()
}

const setupMockShell = () => {
  if (!term || (term as any)._mockShellLoaded) return
  ;(term as any)._mockShellLoaded = true
  
  term.writeln('\x1b[1;33m⚠️ Warning: Local terminal emulation active.\x1b[0m')
  term.writeln('Type "help" for a list of valid commands.\r\n')
  term.write('$ ')
  
  term.onData((data: string) => {
    if (isTerminalConnected.value) {
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ event: 'input', data }))
      }
      return
    }
    
    if (data === '\r') {
      term.writeln('')
      handleCommand(currentLine.trim())
      currentLine = ''
      term.write('$ ')
    } else if (data === '\x7F') { // Backspace
      if (currentLine.length > 0) {
        currentLine = currentLine.slice(0, -1)
        term.write('\b \b')
      }
    } else if (data.charCodeAt(0) < 32) {
      // Ignore controls
    } else {
      currentLine += data
      term.write(data)
    }
  })
}

const handleCommand = (cmd: string) => {
  const parts = cmd.split(' ')
  const mainCmd = parts[0].toLowerCase()
  
  if (mainCmd === 'help') {
    term.writeln('Available commands:')
    term.writeln('  help           - Displays this command helper')
    term.writeln('  neofetch       - Prints OS and hardware stats')
    term.writeln('  docker ps      - Lists container details')
    term.writeln('  vps status     - Shows live metrics')
    term.writeln('  clear          - Clears terminal console')
  } else if (mainCmd === 'neofetch') {
    const isOnline = activeServerDetails.value.status === 'online'
    term.writeln('   \x1b[1;35m.-/+oossssoo+/-.\x1b[0m             \x1b[1;36mpromptops@' + selectedServer.value.toLowerCase().replace(' ', '-') + '\x1b[0m')
    term.writeln('  \x1b[1;35m`:+ssssssssssssssso:`\x1b[0m          -------------------------')
    term.writeln(' \x1b[1;35m-+ssssssssssssssssssyys+-\x1b[0m       OS: ' + activeServerDetails.value.os)
    term.writeln(' \x1b[1;35m.ossssssssssssssssssdMMMNys:\x1b[0m     Host: KVM VPS Cloud Instance')
    term.writeln('\x1b[1;35m/ssssssssssshdhyhhyydMMMMMMMNs.\x1b[0m   Kernel: 6.1.0-21-amd64')
    term.writeln('\x1b[1;35m+ssssssssshqdhyyyyyhdNMMMMMMMMNp\x1b[0m  Uptime: ' + (isOnline ? '5 days, 12 hours' : 'Offline'))
    term.writeln('\x1b[1;35m/ssssssssshdhyyhyyydMMMMMMMNs.\x1b[0m    Shell: bash 5.2.15')
    term.writeln('\x1b[1;35m.ossssssssssssssssssdMMMNys:\x1b[0m      Terminal: xterm.js v3.x')
    term.writeln(' \x1b[1;35m-+ssssssssssssssssssyys+-\x1b[0m       CPU: Intel Xeon Gold 6138 (2) @ 2.00GHz')
    term.writeln('  \x1b[1;35m`:+ssssssssssssssso:`\x1b[0m          Memory: ' + activeServerDetails.value.ram + '% / 4096MB')
    term.writeln('   \x1b[1;35m.-/+oossssoo+/-.\x1b[0m')
  } else if (cmd === 'docker ps') {
    term.writeln('CONTAINER ID   IMAGE               COMMAND                  CREATED         STATUS         PORTS                    NAMES')
    currentContainers.value.forEach((c, idx) => {
      const hex = (100000 + idx).toString(16)
      const statusStr = c.status === 'running' ? 'Up ' + c.uptime : 'Exited (0) 5m ago'
      term.writeln(`${hex}   ${c.image.padEnd(19)} "/docker-entrypoint…"   3 days ago      ${statusStr.padEnd(14)} ${c.ports.padEnd(23)} text-${c.name}`)
    })
  } else if (cmd === 'vps status') {
    term.writeln(`Server: ${selectedServer.value}`)
    term.writeln(`Status: ${activeServerDetails.value.status.toUpperCase()}`)
    term.writeln(`CPU Load: ${activeServerDetails.value.cpu}%`)
    term.writeln(`RAM Usage: ${activeServerDetails.value.ram}%`)
    term.writeln(`OS: ${activeServerDetails.value.os}`)
  } else if (mainCmd === 'clear') {
    term.clear()
  } else if (cmd !== '') {
    term.writeln(`sh: command not found: ${cmd}`)
  }
}

// Watch Theme change to update chart styling
watch(isDark, (darkVal) => {
  if (cpuChart && ramChart) {
    const gridC = darkVal ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)'
    const tickC = darkVal ? '#94a3b8' : '#64748b'

    cpuChart.options.scales.y.grid.color = gridC
    cpuChart.options.scales.y.ticks.color = tickC
    cpuChart.options.scales.x.ticks.color = tickC
    cpuChart.update()

    ramChart.options.scales.y.grid.color = gridC
    ramChart.options.scales.y.ticks.color = tickC
    ramChart.options.scales.x.ticks.color = tickC
    ramChart.update()
  }
})

// Watch Server change to output switcher info in terminal and update charts
watch(selectedServer, (newServ) => {
  servers.value.forEach(s => {
    s.active = s.id === newServ
  })

  // Refill charts with immediate values
  if (cpuChart && ramChart) {
    const targetServ = servers.value.find(s => s.id === newServ)
    const isOnline = targetServ?.status === 'online'
    const valCpu = isOnline ? targetServ?.cpu || 15 : 0
    const valRam = isOnline ? targetServ?.ram || 55 : 0

    cpuChart.data.datasets[0].data = Array(10).fill(valCpu)
    cpuChart.update()

    ramChart.data.datasets[0].data = Array(10).fill(valRam)
    ramChart.update()
  }

  writeTerminalOutput(`\r\n\x1b[1;35m>>> Switched session target: ${newServ}\x1b[0m\r\n$ `)
  addNotification(`Target server: ${newServ}`)
})

// Handle Chat input submit
const submitChat = () => {
  const query = chatInput.value.trim()
  if (!query) return
  
  // Add to user history
  chatHistory.value.push({
    sender: 'user',
    text: query,
    time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  })
  
  chatInput.value = ''
  
  // Simulate AI Response
  setTimeout(() => {
    processAIResponse(query)
  }, 1000)
}

const selectServerCard = (serverName: string) => {
  selectedServer.value = serverName
  const srv = servers.value.find(s => s.name === serverName)
  if (srv) {
    selectedServerId.value = srv.id
    fetchContainers(srv.id)
  }
}

// AI response brain
const processAIResponse = (query: string) => {
  const text = query.toLowerCase()
  let responseText = ''
  
  if (text.includes('hello') || text.includes('hi ')) {
    responseText = `Hello! How can I assist you with **${selectedServer.value}**? You can try asking me to restart docker containers, create backups, or inspect logs.`
  } else if (text.includes('restart')) {
    // Find if a container name is mentioned
    const containersList = currentContainers.value.map(c => c.name)
    const foundContainer = containersList.find(c => text.includes(c))
    
    if (foundContainer) {
      responseText = `Understood. I am going to restart **${foundContainer}** on **${selectedServer.value}**. Sending docker execution details to the terminal stream.`
      restartContainer(foundContainer)
    } else {
      responseText = `Which container would you like to restart on ${selectedServer.value}? Current containers: ${containersList.join(', ')}.`
    }
  } else if (text.includes('backup') || text.includes('/backup')) {
    // Queue new approval
    const servName = text.includes('vps b') ? 'VPS B' : text.includes('vps c') ? 'VPS C' : 'VPS A'
    const newAppId = `app-${Date.now()}`
    
    approvals.value.unshift({
      id: newAppId,
      title: `Create Scheduled Snapshots: ${servName}`,
      server: servName,
      action: 'Volume Snapshot Config',
      file: `/etc/promptops/backup-${servName.toLowerCase().replace(' ', '-')}.json`,
      diff: `+ {\n+   "backup_id": "${newAppId}",\n+   "target": "${servName}",\n+   "storage": "s3://promptops-backups-bucket",\n+   "schedule": "0 0 * * *",\n+   "status": "active"\n+ }`,
      metadata: {
        'Backup Target': `${servName} System Vol`,
        'S3 Destination': 's3://promptops-backups-bucket',
        'Schedule': 'Daily at Midnight',
        'Payload Compression': 'tar.gz (zstd)'
      },
      status: 'pending'
    })
    
    responseText = `I have generated a backup orchestration plan for **${servName}**. You can see the configuration diff in the **Approvals Queue** on the right sidebar. Please review and click Approve to apply the configuration change.`
    addNotification('New approval pending review.')
  } else if (text.includes('status') || text.includes('health')) {
    responseText = `**${selectedServer.value}** is currently **${activeServerDetails.value.status}**. \n- CPU Load: **${activeServerDetails.value.cpu}%**\n- RAM usage: **${activeServerDetails.value.ram}%**\n- Operating System: **${activeServerDetails.value.os}**.`
  } else {
    responseText = `I've analyzed your query: *"${query}"*. Currently, I'm configured to manage the Docker containers on **${selectedServer.value}**, and orchestrate backups. If you want me to write code or modify files, specify the server context or select an item from the sidebar.`
  }
  
  chatHistory.value.push({
    sender: 'ai',
    text: responseText,
    time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  })
}

// Approval decisions
const handleApproval = (id: string, action: 'approve' | 'reject') => {
  const approvalItem = approvals.value.find(a => a.id === id)
  if (!approvalItem) return
  
  if (action === 'approve') {
    approvalItem.status = 'approved'
    addNotification(`Approval ${id} approved.`)
    
    // Output backup tasks to terminal
    writeTerminalOutput(`\r\n$ promptops-agent apply --id ${id}`)
    writeTerminalOutput(`\r\n[INFO] Orchestrating backup routine for ${approvalItem.server}...`)
    writeTerminalOutput(`\r\n[INFO] Reading config from: ${approvalItem.file}`)
    writeTerminalOutput(`\r\n[SUCCESS] Change applied. Backup configuration snapshot saved.`)
    writeTerminalOutput('\r\n$ ')
    
    // Add feedback in chat
    chatHistory.value.push({
      sender: 'ai',
      text: `Approved action **"${approvalItem.title}"**. Change applied successfully in target.`,
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    })
  } else {
    approvalItem.status = 'rejected'
    addNotification(`Approval ${id} rejected.`)
    
    writeTerminalOutput(`\r\n[WARN] Operator rejected configuration change request ${id}.`)
    writeTerminalOutput('\r\n$ ')
    
    chatHistory.value.push({
      sender: 'ai',
      text: `Rejected action **"${approvalItem.title}"**. Execution halted.`,
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    })
  }
}

// Setup onMounted / onUnmounted lifecycle hooks
onMounted(async () => {
  // Add dark class as default
  document.documentElement.classList.add('dark')

  // Load real servers from gateway API
  await fetchServers()

  // Load ChartJS dynamically
  const ChartJS = (await import('chart.js')).Chart
  const registerables = (await import('chart.js')).registerables
  ChartJS.register(...registerables)

  initCharts(ChartJS)
  startChartSimulation()

  // Load xterm.js dynamically
  const { Terminal } = await import('xterm')
  const { FitAddon } = await import('xterm-addon-fit')
  
  const termEl = document.getElementById('terminal-container')
  if (termEl) {
    term = new Terminal({
      cursorBlink: true,
      fontSize: 12,
      fontFamily: 'Courier New, monospace',
      theme: {
        background: '#0f172a', // Slate 900
        foreground: '#e2e8f0', // Slate 200
        cursor: '#6366f1', // Indigo 500
        black: '#0f172a',
        red: '#f43f5e',
        green: '#10b981',
        yellow: '#f59e0b',
        blue: '#3b82f6',
        magenta: '#8b5cf6',
        cyan: '#06b6d4',
        white: '#f8fafc',
      }
    })
    
    fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(termEl)
    fitAddon.fit()
    
    term.writeln('\x1b[1;36mPromptOps Control Terminal v2.1.0\x1b[0m')
    term.writeln('Opening stream connection client...')
    
    connectWebSocket()
  }

  // Handle terminal resizing on window resize
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  if (chartInterval) clearInterval(chartInterval)
  if (socket) socket.close()
  window.removeEventListener('resize', handleResize)
})

const handleResize = () => {
  if (fitAddon) {
    fitAddon.fit()
  }
}
</script>

<template>
  <div :class="[isDark ? 'dark bg-slate-950' : 'bg-slate-50', 'h-screen w-screen overflow-hidden flex flex-col font-sans transition-colors duration-300 relative']">
    
    <!-- Decorative Glowing Elements (Dark Mode Only) -->
    <div v-if="isDark" class="absolute top-[-10%] right-[-10%] w-[600px] h-[600px] rounded-full bg-indigo-900/10 blur-[150px] pointer-events-none z-0"></div>
    <div v-if="isDark" class="absolute bottom-[-10%] left-[-10%] w-[500px] h-[500px] rounded-full bg-purple-900/10 blur-[120px] pointer-events-none z-0"></div>

    <!-- Header bar -->
    <header class="glass h-16 border-b flex-shrink-0 flex items-center justify-between px-6 z-10">
      <div class="flex items-center gap-3">
        <div class="h-8 w-8 rounded-lg bg-gradient-to-tr from-indigo-600 to-purple-600 flex items-center justify-center text-white font-bold font-outfit text-lg shadow-md shadow-indigo-500/20">
          P
        </div>
        <div>
          <span class="font-outfit font-bold tracking-wider text-transparent bg-clip-text bg-gradient-to-r from-indigo-500 to-purple-500 text-lg">
            PROMPTOPS
          </span>
          <span class="hidden sm:inline font-mono text-xs ml-3 text-slate-500 dark:text-slate-400">
            // V4-CONTROL-PANEL
          </span>
        </div>
      </div>

      <!-- Center Status Info -->
      <div class="hidden md:flex items-center gap-6 text-xs font-mono">
        <div class="flex items-center gap-2">
          <span class="h-2 w-2 rounded-full bg-emerald-500 animate-pulse"></span>
          <span class="text-slate-600 dark:text-slate-300">Agent API: <b class="text-emerald-500">Connected</b></span>
        </div>
        <div class="h-4 w-px bg-slate-200 dark:bg-slate-800"></div>
        <div>
          <span class="text-slate-600 dark:text-slate-300">Active server: <b class="text-indigo-500">{{ selectedServer }}</b></span>
        </div>
      </div>

      <!-- Action Utilities -->
      <div class="flex items-center gap-4">
        <!-- Toggle Dark/Light Mode -->
        <button 
          @click="toggleTheme" 
          class="p-2 rounded-xl border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-900 transition-all duration-200 text-slate-600 dark:text-slate-300 cursor-pointer"
          title="Toggle Light/Dark Theme"
        >
          <!-- Sun Icon (Show in Dark) -->
          <svg v-if="isDark" class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364-6.364l-.707.707M6.343 17.657l-.707.707m12.728 0l-.707-.707M6.343 6.343l-.707-.707M12 8a4 4 0 100 8 4 4 0 000-8z" />
          </svg>
          <!-- Moon Icon (Show in Light) -->
          <svg v-else class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
          </svg>
        </button>

        <div class="h-8 w-px bg-slate-200 dark:bg-slate-800"></div>

        <!-- User Quick Profile Mock -->
        <div class="flex items-center gap-2">
          <div class="h-8 w-8 rounded-full bg-gradient-to-br from-indigo-500 to-pink-500 flex items-center justify-center text-xs font-bold text-white uppercase">
            op
          </div>
        </div>
      </div>
    </header>

    <!-- Main Content Area -->
    <main class="flex flex-1 overflow-hidden p-4 gap-4 z-10">

      <!-- Column 1: Left Sidebar (Servers Selection) -->
      <section class="glass rounded-2xl w-72 flex-shrink-0 flex flex-col overflow-hidden p-4">
        <h2 class="font-outfit text-sm font-bold tracking-wider text-slate-400 dark:text-slate-500 uppercase mb-4 flex items-center justify-between">
          <span>SERVERS INDEX</span>
          <span class="bg-indigo-100 dark:bg-indigo-950 text-indigo-600 dark:text-indigo-400 text-[10px] px-2 py-0.5 rounded-full font-mono">
            {{ servers.length }} active
          </span>
        </h2>

        <!-- Servers list -->
        <div class="flex-1 overflow-y-auto space-y-3 pr-1">
          <div v-if="servers.length === 0" class="text-center font-mono text-xs text-slate-500 py-8">No servers registered yet.</div>
          <div 
            v-for="server in servers" 
            :key="server.id"
            @click="selectServerCard(server.name)"
            :class="[
              server.name === selectedServer 
                ? 'border-indigo-500/50 bg-indigo-500/5 dark:bg-indigo-500/10 shadow-md shadow-indigo-500/5 ring-1 ring-indigo-500/30' 
                : 'border-slate-200 dark:border-slate-800/80 hover:bg-slate-100 dark:hover:bg-slate-900/50 hover:border-slate-300 dark:hover:border-slate-800',
              'border rounded-xl p-3.5 transition-all duration-200 cursor-pointer flex flex-col gap-2.5 relative overflow-hidden group'
            ]"
          >
            <!-- Glowing accent on active item -->
            <div v-if="server.name === selectedServer" class="absolute left-0 top-0 bottom-0 w-1 bg-gradient-to-b from-indigo-500 to-purple-600"></div>

            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span :class="[
                  server.status === 'online' ? 'bg-emerald-500 shadow-emerald-500/30' : 'bg-slate-400 shadow-slate-500/20',
                  'h-2 w-2 rounded-full shadow-md'
                ]"></span>
                <span class="font-outfit font-semibold text-sm text-slate-800 dark:text-slate-200">{{ server.name }}</span>
              </div>
              <span class="font-mono text-[10px] uppercase text-slate-500 dark:text-slate-400 bg-slate-100 dark:bg-slate-900/80 px-1.5 py-0.5 rounded">
                {{ server.status }}
              </span>
            </div>

            <!-- Server metadata -->
            <div class="font-mono text-[11px] text-slate-600 dark:text-slate-400 space-y-1">
              <div class="flex justify-between">
                <span>IP Address:</span>
                <span>{{ server.ipAddress }}</span>
              </div>
              <div class="flex justify-between">
                <span>Agent ID:</span>
                <span class="truncate max-w-[120px]">#{{ server.id }}</span>
              </div>
            </div>

            <!-- Micro Metrics (Only if server is online) -->
            <div v-if="server.status === 'online'" class="space-y-1.5 pt-1.5 border-t border-slate-200 dark:border-slate-800/60">
              <div class="flex justify-between items-center text-[10px] font-mono text-slate-500">
                <span>CPU [{{ server.cpu }}%]</span>
                <span>RAM [{{ server.ram }}%]</span>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <!-- CPU bar -->
                <div class="h-1 bg-slate-200 dark:bg-slate-800 rounded-full overflow-hidden">
                  <div 
                    :style="{ width: `${server.cpu}%` }"
                    class="h-full rounded-full transition-all duration-500 bg-gradient-to-r from-indigo-500 to-indigo-400"
                  ></div>
                </div>
                <!-- RAM bar -->
                <div class="h-1 bg-slate-200 dark:bg-slate-800 rounded-full overflow-hidden">
                  <div 
                    :style="{ width: `${server.ram}%` }"
                    class="h-full rounded-full transition-all duration-500 bg-gradient-to-r from-purple-500 to-purple-400"
                  ></div>
                </div>
              </div>
            </div>
            
            <div v-else class="text-center font-mono text-[10px] text-slate-400 dark:text-slate-500 py-1.5 border-t border-dashed border-slate-200 dark:border-slate-800/60">
              ● SERVER HOST OFFLINE
            </div>
          </div>
        </div>

        <!-- Sidebar Footer Server info details -->
        <div class="mt-4 pt-4 border-t border-slate-200 dark:border-slate-800/80 font-mono text-xs text-slate-500 space-y-2">
          <div class="flex items-center justify-between">
            <span>Uptime Metrics:</span>
            <span class="text-slate-700 dark:text-slate-300 font-bold">
              {{ activeServerDetails.status === 'online' ? '99.98%' : 'N/A' }}
            </span>
          </div>
          <div class="flex items-center justify-between">
            <span>Agent Build:</span>
            <span class="text-slate-700 dark:text-slate-300">v1.4-go</span>
          </div>
          
          <button 
            @click="writeTerminalOutput(`\r\n$ ping ${activeServerDetails.ip}\r\nPING ${activeServerDetails.ip} (${activeServerDetails.ip}) 56(84) bytes of data.\r\n64 bytes from ${activeServerDetails.ip}: icmp_seq=1 ttl=64 time=0.85 ms\r\n64 bytes from ${activeServerDetails.ip}: icmp_seq=2 ttl=64 time=0.91 ms\r\n--- ${activeServerDetails.ip} ping statistics ---\r\n2 packets transmitted, 2 received, 0% packet loss, time 1002ms\r\nrtt min/avg/max/mdev = 0.850/0.880/0.910/0.030 ms\r\n$ `)"
            :disabled="activeServerDetails.status !== 'online'"
            class="w-full mt-2 cursor-pointer bg-slate-100 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 py-2 rounded-lg text-slate-700 dark:text-slate-300 font-semibold hover:bg-slate-200 dark:hover:bg-slate-850 active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Ping Server Host
          </button>
        </div>
      </section>

      <!-- Column 2: Central Workspace (Resource monitor, docker panel, terminal drawer) -->
      <section class="flex-1 flex flex-col gap-4 overflow-hidden">
        
        <!-- Top Half: Resource Monitor & Docker Panel -->
        <div class="flex-1 grid grid-rows-2 gap-4 min-h-0 overflow-y-auto pr-1">
          
          <!-- Row 1: Resource Monitor Charts -->
          <div class="glass rounded-2xl p-4 flex flex-col gap-3 min-h-[220px]">
            <div class="flex items-center justify-between border-b border-slate-200 dark:border-slate-800/80 pb-2">
              <h3 class="font-outfit text-sm font-bold tracking-wider text-slate-700 dark:text-slate-300 uppercase flex items-center gap-2">
                <!-- Monitor SVG -->
                <svg class="w-4 h-4 text-indigo-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 002 2h2a2 2 0 002-2z" />
                </svg>
                RESOURCE MONITOR // REAL-TIME METRICS
              </h3>
              <div class="font-mono text-[10px] text-indigo-500 dark:text-indigo-400 bg-indigo-500/10 dark:bg-indigo-500/20 px-2 py-0.5 rounded-full">
                Context: {{ selectedServer }}
              </div>
            </div>
            
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6 flex-1 min-h-0">
              <!-- CPU Chart -->
              <div class="flex flex-col gap-1.5 h-full relative">
                <div class="flex justify-between items-center text-xs font-mono">
                  <span class="text-slate-500 dark:text-slate-400">CPU HISTORY</span>
                  <span class="text-indigo-500 font-semibold">{{ activeServerDetails.cpu }}%</span>
                </div>
                <div class="flex-1 min-h-[120px] bg-slate-900/5 dark:bg-slate-900/40 rounded-xl border border-slate-200 dark:border-slate-800/40 p-2">
                  <canvas id="cpuChart" class="w-full h-full"></canvas>
                </div>
              </div>

              <!-- RAM Chart -->
              <div class="flex flex-col gap-1.5 h-full relative">
                <div class="flex justify-between items-center text-xs font-mono">
                  <span class="text-slate-500 dark:text-slate-400">RAM HISTOGRAM</span>
                  <span class="text-purple-500 font-semibold">{{ activeServerDetails.ram }}%</span>
                </div>
                <div class="flex-1 min-h-[120px] bg-slate-900/5 dark:bg-slate-900/40 rounded-xl border border-slate-200 dark:border-slate-800/40 p-2">
                  <canvas id="ramChart" class="w-full h-full"></canvas>
                </div>
              </div>
            </div>
          </div>

          <!-- Row 2: Docker Containers -->
          <div class="glass rounded-2xl p-4 flex flex-col min-h-[220px] overflow-hidden">
            <div class="flex items-center justify-between border-b border-slate-200 dark:border-slate-800/80 pb-2.5 flex-shrink-0">
              <h3 class="font-outfit text-sm font-bold tracking-wider text-slate-700 dark:text-slate-300 uppercase flex items-center gap-2">
                <!-- Database SVG -->
                <svg class="w-4 h-4 text-purple-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                </svg>
                DOCKER DAEMON CONTAINERS
              </h3>
              
              <button 
                @click="selectedServerId && fetchContainers(selectedServerId)"
                :disabled="!selectedServerId || containersLoading"
                class="font-mono text-xs text-indigo-500 dark:text-indigo-400 hover:underline cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-1"
              >
                <svg v-if="containersLoading" class="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
                </svg>
                {{ containersLoading ? 'Scanning…' : 'Scan Containers' }}
              </button>
            </div>

            <!-- Container cards list -->
            <div class="flex-1 overflow-y-auto mt-3 pr-1 space-y-2">

              <!-- Error state -->
              <div v-if="containersError" class="flex items-center gap-2 text-xs font-mono text-rose-500 bg-rose-500/10 rounded-xl px-4 py-3 border border-rose-500/20">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/></svg>
                Daemon offline or unreachable: {{ containersError }}
              </div>

              <!-- Empty state (no containers) -->
              <div v-else-if="!containersLoading && currentContainers.length === 0" class="text-center font-mono text-xs text-slate-500 py-6">
                {{ selectedServerId ? 'No containers found on this server.' : 'Select a server to view containers.' }}
              </div>

              <!-- Real container cards -->
              <div 
                v-for="(container, idx) in currentContainers" 
                :key="container.id || container.name"
                class="flex items-center justify-between p-3 rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-900/5 dark:bg-slate-900/20 hover:border-slate-300 dark:hover:border-slate-750 transition-colors"
              >
                <div class="flex items-center gap-3">
                  <!-- Container status avatar -->
                  <div :class="[
                    container.status === 'running' ? 'bg-indigo-500/10 text-indigo-500 border-indigo-500/30' : 'bg-slate-200 dark:bg-slate-800 text-slate-500 dark:text-slate-400 border-slate-300 dark:border-slate-700',
                    'h-10 w-10 rounded-lg border flex items-center justify-center flex-shrink-0'
                  ]">
                    <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
                    </svg>
                  </div>

                  <div class="min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-outfit font-semibold text-sm text-slate-800 dark:text-slate-200">{{ container.name }}</span>
                      <span class="font-mono text-[10px] text-slate-500 dark:text-slate-400 px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-900">
                        {{ container.image }}
                      </span>
                    </div>
                    <div class="flex gap-4 font-mono text-[11px] text-slate-600 dark:text-slate-400 mt-1 flex-wrap">
                      <span>Ports: {{ container.ports }}</span>
                      <span>CPU: {{ container.cpu_usage }}</span>
                      <span>Mem: {{ container.memory_usage }}</span>
                    </div>
                  </div>
                </div>

                <div class="flex items-center gap-3">
                  <!-- Status Indicator -->
                  <span :class="[
                    container.status === 'running' ? 'text-emerald-500 bg-emerald-500/10' : 'text-slate-500 bg-slate-150 dark:bg-slate-900',
                    'font-mono text-xs px-2.5 py-1 rounded-full'
                  ]">
                    {{ container.status.toUpperCase() }}
                  </span>

                  <!-- Start / Stop toggle Switch -->
                  <label class="relative inline-flex items-center cursor-pointer select-none">
                    <input 
                      type="checkbox" 
                      :checked="container.status === 'running'"
                      @change="toggleContainer(idx)"
                      class="sr-only peer"
                    >
                    <div class="w-10 h-6 bg-slate-300 dark:bg-slate-800 rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-0.5 after:left-[4px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-indigo-600"></div>
                  </label>

                  <!-- Restart actions button -->
                  <button 
                    @click="restartContainer(container.name)"
                    :disabled="container.status !== 'running'"
                    class="p-2 rounded-lg border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-850 active:scale-95 transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
                    title="Restart Container"
                  >
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 1121.21 8H17" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Bottom Half: Slide-up Web Terminal -->
        <div :class="[
          terminalSize === 'min' ? 'h-11' : terminalSize === 'full' ? 'h-full' : 'h-[320px]',
          'glass rounded-2xl overflow-hidden flex flex-col flex-shrink-0 transition-all duration-300 border border-slate-200 dark:border-slate-800'
        ]">
          <!-- Terminal Header -->
          <div class="h-11 bg-slate-900/60 border-b border-slate-200 dark:border-slate-800/80 flex items-center justify-between px-4 flex-shrink-0">
            <div class="flex items-center gap-2">
              <!-- Console icon -->
              <svg class="w-4 h-4 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
              <span class="font-mono text-xs text-slate-300 tracking-wide font-bold uppercase">
                VPS Terminal Console // {{ selectedServer }}
              </span>
              <!-- Connection State indicator -->
              <span class="flex items-center gap-1 ml-2 text-[10px] font-mono">
                <span :class="[
                  isTerminalConnected ? 'bg-emerald-500' : 'bg-yellow-500 animate-pulse',
                  'h-1.5 w-1.5 rounded-full'
                ]"></span>
                <span class="text-slate-400">{{ isTerminalConnected ? 'Daemon Stream' : 'Mock Mode' }}</span>
              </span>
            </div>

            <!-- Window controls -->
            <div class="flex items-center gap-3">
              <!-- Clear output -->
              <button 
                @click="term?.clear(); term?.write('$ ')"
                class="font-mono text-[10px] text-slate-400 hover:text-white px-2 py-0.5 rounded bg-slate-800 cursor-pointer"
                title="Clear Terminal Display"
              >
                CLEAR
              </button>

              <!-- Size selector icons -->
              <div class="flex items-center gap-1.5 bg-slate-800/50 rounded-lg p-0.5 border border-slate-800">
                <button 
                  @click="terminalSize = 'min'"
                  :class="[terminalSize === 'min' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300', 'p-1 rounded cursor-pointer']"
                  title="Minimize"
                >
                  <span class="block w-2.5 h-0.5 bg-current"></span>
                </button>
                <button 
                  @click="terminalSize = 'half'"
                  :class="[terminalSize === 'half' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300', 'p-1 rounded flex items-center justify-center cursor-pointer']"
                  title="Normal Window"
                >
                  <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M4 12h16" />
                  </svg>
                </button>
                <button 
                  @click="terminalSize = 'full'"
                  :class="[terminalSize === 'full' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300', 'p-1 rounded flex items-center justify-center cursor-pointer']"
                  title="Maximize"
                >
                  <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M4 4h16v16H4V4z" />
                  </svg>
                </button>
              </div>
            </div>
          </div>

          <!-- Terminal Container (rendered via xterm.js) -->
          <div 
            v-show="terminalSize !== 'min'" 
            class="flex-1 bg-slate-900 p-2.5 overflow-hidden relative"
          >
            <div id="terminal-container" class="w-full h-full"></div>
          </div>
        </div>

      </section>

      <!-- Column 3: Right Sidebar (AI Chat Drawer & Approvals Queue) -->
      <section class="glass rounded-2xl w-96 flex-shrink-0 flex flex-col overflow-hidden p-4">
        
        <!-- Tab view for Chat and Approvals -->
        <h2 class="font-outfit text-sm font-bold tracking-wider text-slate-400 dark:text-slate-500 uppercase mb-3 flex items-center gap-1.5 flex-shrink-0">
          <!-- Sparkles SVG -->
          <svg class="w-4 h-4 text-indigo-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" />
          </svg>
          AI DEVOPS AGENT CONSOLE
        </h2>

        <!-- Top Half of right panel: Approvals Queue -->
        <div class="border border-slate-200 dark:border-slate-800/80 rounded-xl p-3 bg-slate-900/5 dark:bg-slate-900/20 mb-4 flex-shrink-0">
          <div class="flex items-center justify-between border-b border-slate-200 dark:border-slate-800/80 pb-2 mb-2">
            <span class="font-mono text-xs font-bold text-slate-600 dark:text-slate-300">APPROVAL GATEWAY</span>
            <span class="bg-indigo-150 dark:bg-indigo-900/40 text-indigo-600 dark:text-indigo-400 font-mono text-[9px] px-2 py-0.5 rounded-full font-bold">
              {{ approvals.filter(a => a.status === 'pending').length }} pending
            </span>
          </div>

          <!-- Approvals list -->
          <div class="space-y-3 max-h-[220px] overflow-y-auto pr-1">
            <div 
              v-for="approval in approvals" 
              :key="approval.id"
              class="border border-slate-200 dark:border-slate-800 rounded-lg p-2.5 bg-white dark:bg-slate-950/60 shadow-sm"
            >
              <div class="flex items-center justify-between text-xs mb-1">
                <span class="font-outfit font-bold text-slate-800 dark:text-slate-200 truncate max-w-[170px]">{{ approval.title }}</span>
                <span :class="[
                  approval.status === 'approved' ? 'text-emerald-500' : approval.status === 'rejected' ? 'text-rose-500' : 'text-amber-500',
                  'font-mono text-[10px] uppercase font-bold'
                ]">{{ approval.status }}</span>
              </div>

              <!-- Metadata table -->
              <div class="grid grid-cols-2 gap-x-2 gap-y-1 font-mono text-[9px] text-slate-500 border-t border-b border-slate-100 dark:border-slate-800/40 py-1.5 my-1.5">
                <div v-for="(val, label) in approval.metadata" :key="label" class="flex justify-between border-r last:border-r-0 border-slate-150 dark:border-slate-800/20 pr-1.5">
                  <span class="text-slate-400 truncate max-w-[80px]">{{ label }}:</span>
                  <span class="text-slate-600 dark:text-slate-300 font-bold truncate max-w-[80px] text-right">{{ val }}</span>
                </div>
              </div>

              <!-- Diff viewer -->
              <div class="bg-slate-950 rounded p-2 overflow-x-auto text-[10px] font-mono text-slate-400 mb-2 leading-relaxed border border-slate-800">
                <pre class="whitespace-pre-wrap">{{ approval.diff }}</pre>
              </div>

              <!-- Action buttons -->
              <div v-if="approval.status === 'pending'" class="flex gap-2 justify-end">
                <button 
                  @click="handleApproval(approval.id, 'reject')"
                  class="cursor-pointer font-mono text-[10px] font-bold text-rose-500 dark:text-rose-400 hover:bg-rose-500/10 px-2.5 py-1 rounded border border-rose-500/20 active:scale-95 transition-all"
                >
                  REJECT
                </button>
                <button 
                  @click="handleApproval(approval.id, 'approve')"
                  class="cursor-pointer font-mono text-[10px] font-bold text-white bg-indigo-600 hover:bg-indigo-500 px-3 py-1 rounded shadow-md shadow-indigo-500/20 active:scale-95 transition-all"
                >
                  APPROVE & APPLY
                </button>
              </div>
            </div>

            <div v-if="approvals.length === 0" class="text-center font-mono text-xs text-slate-400 py-6">
              Queue is completely empty
            </div>
          </div>
        </div>

        <!-- Chat bubble history area -->
        <div class="flex-1 border border-slate-200 dark:border-slate-800/80 rounded-xl p-3 bg-slate-900/5 dark:bg-slate-900/10 flex flex-col min-h-0">
          
          <div class="flex-1 overflow-y-auto space-y-3 mb-3 pr-1" id="chat-messages-container">
            <div 
              v-for="msg in chatHistory" 
              :key="msg.text"
              :class="[
                msg.sender === 'user' ? 'justify-end' : 'justify-start',
                'flex w-full'
              ]"
            >
              <div :class="[
                msg.sender === 'user' 
                  ? 'bg-indigo-600 text-white rounded-br-none shadow-md shadow-indigo-500/5' 
                  : 'bg-white dark:bg-slate-950 border border-slate-200 dark:border-slate-800 text-slate-800 dark:text-slate-200 rounded-bl-none shadow-sm',
                'max-w-[85%] rounded-2xl px-3.5 py-2.5 text-xs flex flex-col gap-1'
              ]">
                <!-- Chat text content -->
                <p class="leading-relaxed whitespace-pre-wrap" v-html="msg.text"></p>
                <!-- Time stamp -->
                <span :class="[
                  msg.sender === 'user' ? 'text-indigo-200' : 'text-slate-400',
                  'text-[9px] font-mono text-right mt-1.5'
                ]">
                  {{ msg.time }}
                </span>
              </div>
            </div>
          </div>

          <!-- Bottom omnibar input -->
          <form @submit.prevent="submitChat" class="flex gap-2 flex-shrink-0">
            <div class="relative flex-1">
              <input 
                type="text" 
                v-model="chatInput"
                placeholder="Ask AI DevOps or run /backup VPS A..." 
                class="w-full bg-white dark:bg-slate-950 border border-slate-200 dark:border-slate-800 rounded-xl px-3.5 py-2.5 text-xs text-slate-850 dark:text-slate-200 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/30 pr-10 font-sans"
              >
              <!-- Sparkle indicators inside input -->
              <span class="absolute right-3.5 top-3 text-slate-400 dark:text-slate-500">
                /
              </span>
            </div>
            <button 
              type="submit"
              class="cursor-pointer bg-indigo-600 hover:bg-indigo-500 text-white p-2.5 rounded-xl flex items-center justify-center active:scale-95 shadow-md shadow-indigo-500/10 transition-all"
            >
              <!-- Send SVG -->
              <svg class="w-4 h-4 transform rotate-90" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
              </svg>
            </button>
          </form>
        </div>

      </section>

    </main>

    <!-- Global Toast Alerts -->
    <div class="absolute bottom-6 right-6 z-50 flex flex-col gap-2 pointer-events-none">
      <transition-group name="list">
        <div 
          v-for="note in notifications" 
          :key="note.id" 
          class="glass-card rounded-xl p-3 border border-indigo-500/20 text-slate-800 dark:text-slate-100 font-mono text-xs flex items-center gap-2.5 pointer-events-auto animate-slide-in"
        >
          <span class="h-2 w-2 rounded-full bg-indigo-500 shadow shadow-indigo-500/50"></span>
          <span>{{ note.text }}</span>
        </div>
      </transition-group>
    </div>

  </div>
</template>

<style>
@import "xterm/css/xterm.css";

/* Extra Transitions & Keyframes */
.theme-transition {
  transition: background-color 0.3s ease, color 0.3s ease, border-color 0.3s ease;
}

/* Animations */
@keyframes slideIn {
  from {
    transform: translateY(20px);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}

.animate-slide-in {
  animation: slideIn 0.3s cubic-bezier(0.16, 1, 0.3, 1) forwards;
}

/* Scrollbar tweaks */
::-webkit-scrollbar {
  width: 5px;
  height: 5px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.15);
  border-radius: 9999px;
}
::-webkit-scrollbar-thumb:hover {
  background: rgba(148, 163, 184, 0.3);
}

.list-enter-active,
.list-leave-active {
  transition: all 0.3s ease;
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: translateX(30px);
}
</style>

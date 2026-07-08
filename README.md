# PromptOps

An AI-driven, multi-VPS Platform-as-a-Service (PaaS) managed through conversational AI agents using the Model Context Protocol (MCP).

## Project Structure (Target MVP)

- `/daemon`: Golang daemon installed on target VPS nodes. Acts as an MCP server.
- `/control-panel`: Nuxt 3 / Next.js web application for server monitoring, registry, and the AI agent chat interface.
- `/orchestrator`: TypeScript AI agent engine using ReAct loop and safety approval gateway.

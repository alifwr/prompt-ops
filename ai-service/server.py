import os
import json
import logging
import asyncio
from concurrent import futures
from typing import Annotated, Sequence, TypedDict, List, Any, Optional

import grpc
from fastapi import FastAPI
from contextlib import asynccontextmanager

from langchain_core.messages import BaseMessage, AIMessage, ToolMessage, HumanMessage
from langchain_core.tools import tool
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.outputs import ChatResult, ChatGeneration
from langchain_openai import ChatOpenAI
from langchain_google_genai import ChatGoogleGenerativeAI
from langgraph.graph import StateGraph, START, END
from langgraph.graph.message import add_messages
from langgraph.checkpoint.memory import MemorySaver

# Import generated gRPC code
import agent_pb2
import agent_pb2_grpc

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s"
)
logger = logging.getLogger("ai_service")

# --- Mock DevOps LLM ---
# This class acts as a rule-based chat model simulating tool-calling behavior.
# It is used when no actual API keys are found, preventing system crashes and
# allowing the orchestrator/gateway flow to be fully verified.
class MockLLM(BaseChatModel):
    def _generate(
        self,
        messages: List[BaseMessage],
        stop: Optional[List[str]] = None,
        run_manager: Optional[Any] = None,
        **kwargs: Any,
    ) -> ChatResult:
        # Get the last user message
        last_message = messages[-1]
        content = last_message.content.lower().strip()
        
        tool_calls = []
        response_text = ""
        
        # Simple rule-based mock tool triggers
        if "system stats" in content or "stats" in content:
            tool_calls = [{
                "name": "get_system_stats",
                "args": {},
                "id": "call_mock_stats"
            }]
            response_text = "Let me query the target VPS system statistics."
        elif "list container" in content or "show container" in content:
            tool_calls = [{
                "name": "list_containers",
                "args": {},
                "id": "call_mock_list"
            }]
            response_text = "Querying target VPS for docker containers..."
        elif "stop container" in content or "start container" in content or "restart container" in content:
            action = "stop" if "stop" in content else ("start" if "start" in content else "restart")
            container_id = "web"
            words = content.split()
            if words[-1] != action:
                container_id = words[-1]
            tool_calls = [{
                "name": "control_container",
                "args": {"container_id": container_id, "action": action},
                "id": "call_mock_control"
            }]
            response_text = f"Requesting action '{action}' on container '{container_id}'."
        elif "deploy" in content or "compose" in content:
            tool_calls = [{
                "name": "deploy_compose",
                "args": {
                    "compose_content": "version: '3'\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - '80:80'",
                    "project_name": "nginx-web"
                },
                "id": "call_mock_deploy"
            }]
            response_text = "Preparing to deploy docker compose stack..."
        elif "backup" in content:
            tool_calls = [{
                "name": "backup_database",
                "args": {
                    "container_name": "db-postgres",
                    "db_type": "postgres",
                    "backup_path": "/backups/db_backup.sql"
                },
                "id": "call_mock_backup"
            }]
            response_text = "Preparing database backup operation."
        elif "token" in content or "register" in content or "add vps" in content:
            tool_calls = [{
                "name": "generate_vps_token",
                "args": {"name": "new-vps-server"},
                "id": "call_mock_token"
            }]
            response_text = "I will generate a registration token for the new VPS."
        elif "domain" in content or "ssl" in content or "caddy" in content:
            tool_calls = [{
                "name": "configure_domain",
                "args": {"domain": "example.com", "email": "admin@example.com", "project_name": "web"},
                "id": "call_mock_domain"
            }]
            response_text = "Preparing to configure domain and SSL."
        else:
            # Check if there is a tool response to summarize
            tool_messages = [m for m in messages if isinstance(m, ToolMessage)]
            if tool_messages:
                last_tool_msg = tool_messages[-1]
                response_text = f"Result of execution: {last_tool_msg.content}"
            else:
                response_text = "Hello! I am the DevOps orchestrator assistant. I can help with 'stats', 'list containers', 'stop/start/restart container <name>', 'deploy compose', or 'backup database'."
                
        ai_message = AIMessage(content=response_text, tool_calls=tool_calls)
        return ChatResult(generations=[ChatGeneration(message=ai_message)])

    @property
    def _llm_type(self) -> str:
        return "mock-devops-llm"
        
    def bind_tools(self, tools: list, **kwargs: Any) -> "MockLLM":
        return self

# --- Define the Tools ---
@tool
def get_system_stats():
    """Get system resource usage and stats (CPU, Memory, Disk, etc.)."""
    return "get_system_stats placeholder"

@tool
def list_containers():
    """List all Docker containers running or stopped on the server."""
    return "list_containers placeholder"

@tool
def control_container(container_id: str, action: str):
    """Control a Docker container (start, stop, restart)."""
    return "control_container placeholder"

@tool
def deploy_compose(compose_content: str, project_name: str):
    """Deploy a Docker Compose stack."""
    return "deploy_compose placeholder"

@tool
def backup_database(container_name: str, db_type: str, backup_path: str):
    """Backup a database running in a container."""
    return "backup_database placeholder"

@tool
def generate_vps_token(name: str):
    """Generate a registration token and one-liner installer script to add a new VPS to the cluster."""
    return "generate_vps_token placeholder"

@tool
def configure_domain(domain: str, email: str, project_name: str):
    """Configure a domain with Let's Encrypt SSL using Caddy for a specific deployed project."""
    return "configure_domain placeholder"

tools = [get_system_stats, list_containers, control_container, deploy_compose, backup_database, generate_vps_token, configure_domain]

# --- Initialize Model ---
openai_api_key = os.getenv("OPENAI_API_KEY")
gemini_api_key = os.getenv("GEMINI_API_KEY") or os.getenv("GOOGLE_API_KEY")

if False:
    pass
elif False:
    pass
else:
    logger.info("No API keys found or explicitly ignored. Running with MockLLM.")
    llm = MockLLM().bind_tools(tools)

# --- Define LangGraph State & Graph ---
class AgentState(TypedDict):
    messages: Annotated[list[BaseMessage], add_messages]

def agent_node(state: AgentState):
    messages = state["messages"]
    response = llm.invoke(messages)
    return {"messages": [response]}

def tools_node(state: AgentState):
    # This node is a placeholder bypassed by the manual interrupt step.
    return state

def route_model(state: AgentState):
    last_message = state["messages"][-1]
    if isinstance(last_message, AIMessage) and last_message.tool_calls:
        return "tools"
    return END

# Build the Graph
builder = StateGraph(AgentState)
builder.add_node("agent", agent_node)
builder.add_node("tools", tools_node)

builder.add_conditional_edges("agent", route_model)
builder.add_edge("tools", "agent")
builder.add_edge(START, "agent")

# Compile with a Checkpointer and breakpoint before the tools node
memory_saver = MemorySaver()
graph = builder.compile(checkpointer=memory_saver, interrupt_before=["tools"])

# Helper to generate human-friendly explanations for safety/approvals
def generate_tool_description(name: str, args: dict) -> str:
    if name == "get_system_stats":
        return "Retrieve system resource stats (CPU, memory, disk, network) for the server."
    elif name == "list_containers":
        return "List all Docker containers currently running or stopped on the server."
    elif name == "control_container":
        action = args.get("action", "manage")
        container = args.get("container_id", "unknown")
        return f"Perform action '{action}' on container '{container}'."
    elif name == "deploy_compose":
        project = args.get("project_name", "unknown")
        return f"Deploy docker-compose project '{project}' to the server."
    elif name == "backup_database":
        db = args.get("db_type", "database")
        container = args.get("container_name", "unknown")
        path = args.get("backup_path", "unknown")
        return f"Create a '{db}' backup of database container '{container}' and save it to '{path}'."
    elif name == "generate_vps_token":
        vps_name = args.get("name", "new-vps")
        return f"Generate a registration token and installer script for new VPS: '{vps_name}'."
    elif name == "configure_domain":
        domain = args.get("domain", "unknown")
        project = args.get("project_name", "unknown")
        return f"Configure domain '{domain}' with SSL for project '{project}'."
    return f"Execute tool '{name}' with arguments: {args}"

# --- gRPC Service Implementation ---
class AgentServiceServicer(agent_pb2_grpc.AgentServiceServicer):
    def Chat(self, request: agent_pb2.ChatRequest, context: grpc.ServicerContext) -> agent_pb2.ChatResponse:
        session_id = request.session_id
        message = request.message
        server_id = request.server_id
        
        logger.info(f"[gRPC Chat] Session: {session_id}, Server: {server_id}, Msg: {message}")
        
        config = {"configurable": {"thread_id": session_id}}
        
        # Check if the message is a tool response (approved execution or rejection)
        is_tool_response = False
        tool_result_content = ""
        
        if message.startswith("Approved. Tool Result:"):
            is_tool_response = True
            tool_result_content = message[len("Approved. Tool Result:"):].strip()
        elif message.startswith("USER REJECTED"):
            is_tool_response = True
            tool_result_content = f"Error: {message.strip()}"
            
        try:
            if is_tool_response:
                # Retrieve the current state of the graph
                state = graph.get_state(config)
                if state.next and "tools" in state.next:
                    # Retrieve the last AIMessage containing the tool calls
                    last_message = state.values["messages"][-1]
                    if isinstance(last_message, AIMessage) and last_message.tool_calls:
                        tool_call = last_message.tool_calls[0]
                        tool_call_id = tool_call["id"]
                        tool_name = tool_call["name"]
                        
                        logger.info(f"[gRPC Chat] Resuming graph with tool response for tool_call_id={tool_call_id}, tool={tool_name}")
                        
                        # Update state with the ToolMessage as the 'tools' node output
                        tool_msg = ToolMessage(
                            content=tool_result_content,
                            name=tool_name,
                            tool_call_id=tool_call_id
                        )
                        graph.update_state(config, {"messages": [tool_msg]}, as_node="tools")
                        
                        # Resume graph execution
                        graph.invoke(None, config)
                    else:
                        logger.warning("Graph was in 'tools' state but last message had no tool calls. Appending message normally.")
                        graph.invoke({"messages": [HumanMessage(content=message)]}, config)
                else:
                    logger.warning("Received tool result message but graph was not in 'tools' state. Appending message normally.")
                    graph.invoke({"messages": [HumanMessage(content=message)]}, config)
            else:
                # Normal human input
                logger.info("Invoking graph with new human input.")
                graph.invoke({"messages": [HumanMessage(content=message)]}, config)
            
            # Retrieve state after execution/resume
            updated_state = graph.get_state(config)
            
            # Check if graph paused on a breakpoint before tools execution
            if updated_state.next and "tools" in updated_state.next:
                last_message = updated_state.values["messages"][-1]
                if isinstance(last_message, AIMessage) and last_message.tool_calls:
                    proposed_calls = []
                    for tc in last_message.tool_calls:
                        name = tc["name"]
                        args = tc["args"]
                        args_json = json.dumps(args)
                        desc = generate_tool_description(name, args)
                        
                        logger.info(f"[gRPC Chat] Yielding proposed tool call: {name}")
                        
                        proposed_calls.append(agent_pb2.ToolCall(
                            name=name,
                            arguments_json=args_json,
                            description=desc
                        ))
                    
                    return agent_pb2.ChatResponse(
                        response_text=last_message.content or "Manual approval required to execute system action.",
                        is_approval_required=True,
                        proposed_tool_calls=proposed_calls
                    )
            
            # Graph finished or did not call tools
            last_message = updated_state.values["messages"][-1]
            return agent_pb2.ChatResponse(
                response_text=last_message.content,
                is_approval_required=False,
                proposed_tool_calls=[]
            )
            
        except Exception as e:
            logger.exception("Error occurred in gRPC Chat method handling")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return agent_pb2.ChatResponse()

# --- FastAPI Integration ---
grpc_server = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    global grpc_server
    
    # Initialize gRPC server
    grpc_server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    agent_pb2_grpc.add_AgentServiceServicer_to_server(AgentServiceServicer(), grpc_server)
    
    # Bind to port 50051
    port = "50051"
    grpc_server.add_insecure_port(f"0.0.0.0:{port}")
    grpc_server.start()
    logger.info(f"gRPC server started, listening on port {port}")
    
    yield
    
    # Graceful shutdown
    if grpc_server:
        logger.info("Stopping gRPC server...")
        grpc_server.stop(0)
        logger.info("gRPC server stopped.")

app = FastAPI(lifespan=lifespan)

@app.get("/health")
def health_check():
    return {"status": "healthy", "service": "ai-orchestrator"}

if __name__ == "__main__":
    import uvicorn
    logger.info("Starting FastAPI/uvicorn server...")
    uvicorn.run("server:app", host="0.0.0.0", port=8000, log_level="info")

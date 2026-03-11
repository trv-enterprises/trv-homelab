"""
FastAPI Server

Web API and WebSocket server for MCP client frontend.

License: Mozilla Public License 2.0
"""

import json
import logging
import os
from typing import Dict, List
from fastapi import FastAPI, WebSocket, WebSocketDisconnect, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

from mcp_client import MCPSSEClient
from llm_integration import ClaudeIntegration

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI
app = FastAPI(title="EdgeLake MCP Client API")

# Enable CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # In production, specify frontend URL
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Global state
mcp_client: MCPSSEClient = None
claude: ClaudeIntegration = None
conversation_history: List[Dict[str, any]] = []


class ChatMessage(BaseModel):
    """Chat message model."""
    message: str


@app.on_event("startup")
async def startup_event():
    """Initialize MCP client and Claude on startup."""
    global mcp_client, claude

    # Get configuration from environment
    mcp_host = os.getenv("MCP_HOST", "<edgelake-lan-ip>")
    mcp_port = int(os.getenv("MCP_PORT", "50051"))
    anthropic_api_key = os.getenv("ANTHROPIC_API_KEY")

    if not anthropic_api_key:
        logger.error("ANTHROPIC_API_KEY environment variable not set!")
        return

    # Initialize MCP client
    logger.info(f"Connecting to MCP server at {mcp_host}:{mcp_port}")
    mcp_client = MCPSSEClient(host=mcp_host, port=mcp_port)

    if await mcp_client.connect():
        logger.info(f"Connected to MCP server. Discovered {len(mcp_client.get_tools())} tools")
    else:
        logger.error("Failed to connect to MCP server")
        return

    # Initialize Claude
    claude = ClaudeIntegration(api_key=anthropic_api_key)
    logger.info("Claude integration initialized")


@app.get("/")
async def root():
    """Root endpoint."""
    return {"message": "EdgeLake MCP Client API", "status": "running"}


@app.get("/tools")
async def get_tools():
    """Get available MCP tools."""
    if not mcp_client:
        raise HTTPException(status_code=503, detail="MCP client not initialized")

    tools = mcp_client.get_tools()
    return {"tools": tools}


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {
        "status": "healthy",
        "mcp_connected": mcp_client is not None,
        "claude_available": claude is not None,
        "tools_count": len(mcp_client.get_tools()) if mcp_client else 0
    }


@app.websocket("/ws")
async def websocket_endpoint(websocket: WebSocket):
    """
    WebSocket endpoint for real-time chat.

    Client sends: {"message": "user message"}
    Server streams: {"type": "...", "data": {...}}
    """
    await websocket.accept()
    logger.info("WebSocket connection established")

    try:
        while True:
            # Receive message from client
            data = await websocket.receive_json()
            user_message = data.get("message", "")

            if not user_message:
                await websocket.send_json({"type": "error", "data": {"error": "Empty message"}})
                continue

            logger.info(f"Received message: {user_message}")

            # Send acknowledgment
            await websocket.send_json({"type": "message_received", "data": {"message": user_message}})

            # Get available tools
            mcp_tools = mcp_client.get_tools() if mcp_client else []

            # Tool executor function
            async def execute_tool(tool_name: str, tool_input: Dict):
                """Execute MCP tool."""
                logger.info(f"Executing tool: {tool_name} with input: {tool_input}")
                return await mcp_client.call_tool(tool_name, tool_input)

            # Stream response from Claude with tool execution
            async for event in claude.chat_with_tools(
                user_message=user_message,
                messages=conversation_history,
                tools=mcp_tools,
                tool_executor=execute_tool
            ):
                # Send event to client
                await websocket.send_json({"type": event["type"], "data": event})

                # Handle special events
                if event["type"] == "conversation_complete":
                    logger.info("Conversation turn complete")

                elif event["type"] == "error":
                    logger.error(f"Error during chat: {event.get('error')}")

            # Update conversation history
            # (This is simplified - in production, track full conversation state)
            conversation_history.append({"role": "user", "content": user_message})

    except WebSocketDisconnect:
        logger.info("WebSocket connection closed")
    except Exception as e:
        logger.error(f"WebSocket error: {e}", exc_info=True)
        try:
            await websocket.send_json({"type": "error", "data": {"error": str(e)}})
        except:
            pass


@app.post("/chat")
async def chat(message: ChatMessage):
    """
    Alternative HTTP endpoint for chat (non-streaming).

    Args:
        message: Chat message

    Returns:
        Complete response
    """
    if not claude or not mcp_client:
        raise HTTPException(status_code=503, detail="Services not initialized")

    response_text = ""
    tool_calls = []

    # Get available tools
    mcp_tools = mcp_client.get_tools()

    # Tool executor
    async def execute_tool(tool_name: str, tool_input: Dict):
        return await mcp_client.call_tool(tool_name, tool_input)

    # Stream and collect response
    async for event in claude.chat_with_tools(
        user_message=message.message,
        messages=conversation_history,
        tools=mcp_tools,
        tool_executor=execute_tool
    ):
        if event["type"] == "text_delta":
            response_text += event["text"]
        elif event["type"] == "tool_result":
            tool_calls.append({
                "tool": event["tool_name"],
                "result": event["result"]
            })

    return {
        "response": response_text,
        "tool_calls": tool_calls
    }


if __name__ == "__main__":
    import uvicorn

    # Run server
    uvicorn.run(
        "api_server:app",
        host="0.0.0.0",
        port=8000,
        reload=True,
        log_level="info"
    )

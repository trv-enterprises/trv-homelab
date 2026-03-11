"""
MCP SSE Client

Connects to EdgeLake MCP server via Server-Sent Events (SSE) transport.
Handles tool discovery and execution using MCP protocol (JSON-RPC 2.0).

License: Mozilla Public License 2.0
"""

import json
import logging
from typing import Any, Dict, List, Optional
import httpx
import asyncio

logger = logging.getLogger(__name__)


class MCPSSEClient:
    """
    MCP client that connects to an MCP server via SSE transport.
    """

    def __init__(self, host: str, port: int):
        """
        Initialize MCP SSE client.

        Args:
            host: MCP server hostname/IP
            port: MCP server port
        """
        self.host = host
        self.port = port
        self.base_url = f"http://{host}:{port}"
        self.session_id: Optional[str] = None
        self.endpoint_url: Optional[str] = None
        self.tools: List[Dict[str, Any]] = []
        self.message_id = 0

    async def connect(self) -> bool:
        """
        Connect to MCP server and establish session.

        Returns:
            bool: True if connection successful
        """
        try:
            logger.info(f"Connecting to MCP server at {self.base_url}/sse")

            async with httpx.AsyncClient() as client:
                # Initial SSE connection to get session info
                async with client.stream("GET", f"{self.base_url}/sse") as response:
                    if response.status_code != 200:
                        logger.error(f"Failed to connect: HTTP {response.status_code}")
                        return False

                    # Read SSE events
                    async for line in response.aiter_lines():
                        if line.startswith("data: "):
                            data = json.loads(line[6:])  # Remove "data: " prefix

                            if "endpoint" in data:
                                self.endpoint_url = data["endpoint"]
                                logger.info(f"Received endpoint: {self.endpoint_url}")
                                break

            if not self.endpoint_url:
                logger.error("No endpoint received from server")
                return False

            # Initialize session
            await self._initialize_session()

            # Discover tools
            await self._discover_tools()

            logger.info(f"Connected successfully. Discovered {len(self.tools)} tools")
            return True

        except Exception as e:
            logger.error(f"Connection failed: {e}", exc_info=True)
            return False

    async def _initialize_session(self):
        """Initialize MCP session with server."""
        init_request = {
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {
                    "roots": {"listChanged": True},
                    "sampling": {}
                },
                "clientInfo": {
                    "name": "edgelake-mcp-client",
                    "version": "0.1.0"
                }
            }
        }

        response = await self._send_request(init_request)
        logger.debug(f"Initialize response: {response}")

        # Send initialized notification
        initialized_notification = {
            "jsonrpc": "2.0",
            "method": "notifications/initialized"
        }

        await self._send_request(initialized_notification, expect_response=False)

    async def _discover_tools(self):
        """Discover available tools from MCP server."""
        tools_request = {
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "tools/list"
        }

        response = await self._send_request(tools_request)

        if "result" in response and "tools" in response["result"]:
            self.tools = response["result"]["tools"]
            logger.info(f"Discovered tools: {[t['name'] for t in self.tools]}")
        else:
            logger.warning(f"No tools found in response: {response}")

    async def call_tool(self, tool_name: str, arguments: Dict[str, Any]) -> Any:
        """
        Execute a tool on the MCP server.

        Args:
            tool_name: Name of the tool to execute
            arguments: Tool arguments

        Returns:
            Tool execution result
        """
        tool_request = {
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "tools/call",
            "params": {
                "name": tool_name,
                "arguments": arguments
            }
        }

        logger.info(f"Calling tool '{tool_name}' with arguments: {arguments}")
        response = await self._send_request(tool_request)

        if "result" in response:
            # Extract text content from MCP response
            content = response["result"].get("content", [])
            if content and len(content) > 0:
                text_content = content[0].get("text", "")
                try:
                    # Try to parse as JSON
                    return json.loads(text_content)
                except json.JSONDecodeError:
                    return text_content
            return response["result"]
        elif "error" in response:
            logger.error(f"Tool call error: {response['error']}")
            raise Exception(f"Tool call failed: {response['error']}")

        return response

    async def _send_request(self, request: Dict[str, Any], expect_response: bool = True) -> Dict[str, Any]:
        """
        Send JSON-RPC request to MCP server.

        Args:
            request: JSON-RPC request object
            expect_response: Whether to wait for a response

        Returns:
            JSON-RPC response object
        """
        async with httpx.AsyncClient(timeout=30.0) as client:
            response = await client.post(
                self.endpoint_url,
                json=request,
                headers={"Content-Type": "application/json"}
            )

            if not expect_response:
                return {}

            if response.status_code != 200:
                raise Exception(f"HTTP {response.status_code}: {response.text}")

            return response.json()

    def _next_id(self) -> int:
        """Generate next message ID."""
        self.message_id += 1
        return self.message_id

    def get_tools(self) -> List[Dict[str, Any]]:
        """
        Get list of available tools.

        Returns:
            List of tool definitions
        """
        return self.tools

    def get_tool_by_name(self, name: str) -> Optional[Dict[str, Any]]:
        """
        Get tool definition by name.

        Args:
            name: Tool name

        Returns:
            Tool definition or None
        """
        for tool in self.tools:
            if tool["name"] == name:
                return tool
        return None


async def main():
    """Test MCP client."""
    logging.basicConfig(level=logging.INFO)

    # Connect to EdgeLake MCP server
    client = MCPSSEClient(host="<edgelake-lan-ip>", port=50051)

    if await client.connect():
        print(f"\nConnected! Available tools:")
        for tool in client.get_tools():
            print(f"  - {tool['name']}: {tool.get('description', 'No description')}")

        # Test list_databases tool
        if client.get_tool_by_name("list_databases"):
            print("\nTesting list_databases tool...")
            result = await client.call_tool("list_databases", {})
            print(f"Result: {result}")
    else:
        print("Failed to connect to MCP server")


if __name__ == "__main__":
    asyncio.run(main())

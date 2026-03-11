"""
Anthropic Claude Integration

Integrates Claude with MCP tools for EdgeLake queries.

License: Mozilla Public License 2.0
"""

import json
import logging
from typing import Any, AsyncGenerator, Dict, List, Optional
from anthropic import Anthropic, AsyncAnthropic
from anthropic.types import MessageStreamEvent

logger = logging.getLogger(__name__)


class ClaudeIntegration:
    """
    Manages Claude API interactions with MCP tool integration.
    """

    def __init__(self, api_key: str, model: str = "claude-3-5-sonnet-20241022"):
        """
        Initialize Claude integration.

        Args:
            api_key: Anthropic API key
            model: Claude model to use
        """
        self.client = AsyncAnthropic(api_key=api_key)
        self.model = model
        self.max_tokens = 4096

    def mcp_tools_to_anthropic_tools(self, mcp_tools: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """
        Convert MCP tool definitions to Anthropic tool format.

        Args:
            mcp_tools: List of MCP tool definitions

        Returns:
            List of Anthropic tool definitions
        """
        anthropic_tools = []

        for tool in mcp_tools:
            anthropic_tool = {
                "name": tool["name"],
                "description": tool.get("description", ""),
                "input_schema": tool.get("inputSchema", {"type": "object", "properties": {}})
            }
            anthropic_tools.append(anthropic_tool)

        return anthropic_tools

    async def chat(
        self,
        messages: List[Dict[str, str]],
        tools: List[Dict[str, Any]],
        system: Optional[str] = None
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """
        Stream chat with Claude, supporting tool use.

        Args:
            messages: Conversation messages [{"role": "user"|"assistant", "content": "..."}]
            tools: Available tools in Anthropic format
            system: System prompt

        Yields:
            Events: text chunks, tool use requests, etc.
        """
        try:
            # Convert MCP tools to Anthropic format
            anthropic_tools = self.mcp_tools_to_anthropic_tools(tools)

            # Default system prompt
            if not system:
                system = "You are a helpful AI assistant with access to EdgeLake database tools. Use the available tools to answer questions about data."

            logger.info(f"Starting chat with {len(messages)} messages and {len(anthropic_tools)} tools")

            # Call Claude API with streaming
            async with self.client.messages.stream(
                model=self.model,
                max_tokens=self.max_tokens,
                system=system,
                messages=messages,
                tools=anthropic_tools
            ) as stream:
                async for event in stream:
                    yield self._process_stream_event(event)

        except Exception as e:
            logger.error(f"Chat error: {e}", exc_info=True)
            yield {"type": "error", "error": str(e)}

    def _process_stream_event(self, event: MessageStreamEvent) -> Dict[str, Any]:
        """
        Process streaming event from Claude.

        Args:
            event: Anthropic stream event

        Returns:
            Processed event dict
        """
        event_type = event.type

        if event_type == "message_start":
            return {"type": "message_start"}

        elif event_type == "content_block_start":
            block = event.content_block
            if block.type == "text":
                return {"type": "text_start"}
            elif block.type == "tool_use":
                return {
                    "type": "tool_use_start",
                    "tool_use_id": block.id,
                    "tool_name": block.name
                }

        elif event_type == "content_block_delta":
            delta = event.delta
            if delta.type == "text_delta":
                return {"type": "text_delta", "text": delta.text}
            elif delta.type == "input_json_delta":
                return {"type": "tool_input_delta", "json": delta.partial_json}

        elif event_type == "content_block_stop":
            return {"type": "content_block_stop"}

        elif event_type == "message_delta":
            return {"type": "message_delta", "stop_reason": event.delta.stop_reason}

        elif event_type == "message_stop":
            return {"type": "message_stop"}

        return {"type": "unknown", "raw": str(event)}

    async def chat_with_tools(
        self,
        user_message: str,
        messages: List[Dict[str, Any]],
        tools: List[Dict[str, Any]],
        tool_executor: Any
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """
        Chat with automatic tool execution.

        Args:
            user_message: User's message
            messages: Conversation history
            tools: Available MCP tools
            tool_executor: Function to execute tools (async callable)

        Yields:
            Stream events including text and tool execution results
        """
        # Add user message
        conversation = messages.copy()
        conversation.append({"role": "user", "content": user_message})

        max_iterations = 5  # Prevent infinite loops
        iteration = 0

        while iteration < max_iterations:
            iteration += 1

            tool_uses = []
            current_text = ""

            # Stream response from Claude
            async for event in self.chat(conversation, tools):
                yield event

                if event["type"] == "text_delta":
                    current_text += event["text"]

                elif event["type"] == "tool_use_start":
                    tool_uses.append({
                        "id": event["tool_use_id"],
                        "name": event["tool_name"],
                        "input": ""
                    })

                elif event["type"] == "tool_input_delta":
                    if tool_uses:
                        tool_uses[-1]["input"] += event["json"]

                elif event["type"] == "message_stop":
                    # Process tool uses if any
                    if tool_uses:
                        yield {"type": "tool_execution_start", "tool_count": len(tool_uses)}

                        # Execute each tool
                        tool_results = []
                        for tool_use in tool_uses:
                            try:
                                # Parse tool input
                                tool_input = json.loads(tool_use["input"])

                                yield {
                                    "type": "tool_executing",
                                    "tool_name": tool_use["name"],
                                    "tool_input": tool_input
                                }

                                # Execute tool
                                result = await tool_executor(tool_use["name"], tool_input)

                                yield {
                                    "type": "tool_result",
                                    "tool_name": tool_use["name"],
                                    "result": result
                                }

                                # Format result for Claude
                                tool_results.append({
                                    "type": "tool_result",
                                    "tool_use_id": tool_use["id"],
                                    "content": json.dumps(result) if not isinstance(result, str) else result
                                })

                            except Exception as e:
                                logger.error(f"Tool execution error: {e}", exc_info=True)
                                tool_results.append({
                                    "type": "tool_result",
                                    "tool_use_id": tool_use["id"],
                                    "content": f"Error: {str(e)}",
                                    "is_error": True
                                })

                        # Add assistant message with tool uses
                        assistant_content = []
                        if current_text:
                            assistant_content.append({"type": "text", "text": current_text})
                        for tool_use in tool_uses:
                            assistant_content.append({
                                "type": "tool_use",
                                "id": tool_use["id"],
                                "name": tool_use["name"],
                                "input": json.loads(tool_use["input"])
                            })

                        conversation.append({
                            "role": "assistant",
                            "content": assistant_content
                        })

                        # Add tool results
                        conversation.append({
                            "role": "user",
                            "content": tool_results
                        })

                        # Continue conversation with tool results
                        break
                    else:
                        # No tool use, conversation complete
                        yield {"type": "conversation_complete"}
                        return

            if not tool_uses:
                break

        yield {"type": "conversation_complete"}


async def main():
    """Test Claude integration."""
    import os
    logging.basicConfig(level=logging.INFO)

    api_key = os.getenv("ANTHROPIC_API_KEY")
    if not api_key:
        print("Please set ANTHROPIC_API_KEY environment variable")
        return

    claude = ClaudeIntegration(api_key=api_key)

    # Test simple chat
    messages = []
    tools = []

    print("Testing Claude chat...")
    async for event in claude.chat(
        messages=[{"role": "user", "content": "What is EdgeLake?"}],
        tools=tools
    ):
        if event["type"] == "text_delta":
            print(event["text"], end="", flush=True)

    print("\n\nDone!")


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())

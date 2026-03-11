# CLAUDE.md

This file provides guidance to Claude Code when working with the EdgeLake MCP Client codebase.

## Project Overview

The EdgeLake MCP Client is a full-stack application that connects Anthropic's Claude AI to EdgeLake's distributed database network via the Model Context Protocol (MCP). It enables natural language queries against EdgeLake data through a modern chat interface.

## Architecture

```
┌─────────────────────────┐
│   React Frontend        │  Port 5173 (dev)
│   (TypeScript + Vite)   │  Browser-based chat UI
└───────────┬─────────────┘
            │ WebSocket
            ▼
┌─────────────────────────┐
│   FastAPI Backend       │  Port 8000
│   (Python 3.11+)        │  WebSocket + REST API
└───────┬─────────┬───────┘
        │         │
        │         └──────────► Anthropic Claude API
        │                      (claude-3-5-sonnet)
        ▼
┌─────────────────────────┐
│   MCP SSE Client        │  Custom Python client
│   (httpx + asyncio)     │  MCP Protocol (JSON-RPC 2.0)
└───────────┬─────────────┘
            │ Server-Sent Events (SSE)
            ▼
┌─────────────────────────┐
│ EdgeLake MCP Server     │  Port 50051
│ (edge_lake/mcp_server)  │  SSE transport
└───────────┬─────────────┘
            │ Direct Python API calls
            ▼
┌─────────────────────────┐
│   EdgeLake Network      │  Distributed database
│   (Query/Operator nodes)│  Real-time data queries
└─────────────────────────┘
```

## Key Components

### Backend (`backend/`)

#### `mcp_client.py`
**Purpose**: MCP protocol client for connecting to EdgeLake MCP server

**Key Classes**:
- `MCPSSEClient`: Main client class
  - `connect()`: Establishes SSE connection, initializes session, discovers tools
  - `call_tool(tool_name, arguments)`: Executes MCP tools
  - `get_tools()`: Returns list of available tools

**MCP Protocol Flow**:
1. Connect to SSE endpoint (`GET /sse`)
2. Receive endpoint URL in SSE stream
3. Send `initialize` request (JSON-RPC 2.0)
4. Send `notifications/initialized` notification
5. Call `tools/list` to discover available tools
6. Call `tools/call` to execute tools

**Important**: Uses `httpx.AsyncClient` for all HTTP operations. All methods are async.

#### `llm_integration.py`
**Purpose**: Anthropic Claude integration with automatic tool execution

**Key Classes**:
- `ClaudeIntegration`: Manages Claude API interactions
  - `mcp_tools_to_anthropic_tools()`: Converts MCP tool format to Anthropic format
  - `chat()`: Basic streaming chat
  - `chat_with_tools()`: Chat with automatic tool execution (agentic loop)

**Tool Execution Flow**:
1. User sends message
2. Claude receives message + available tools
3. Claude may request tool execution (via tool_use blocks)
4. Backend executes tool via MCP client
5. Tool results sent back to Claude
6. Claude formulates response with results
7. Process repeats if Claude needs more tools (max 5 iterations)

**Streaming Events**:
- `text_delta`: Streaming text from Claude
- `tool_use_start`: Claude requesting tool execution
- `tool_executing`: Tool is being executed
- `tool_result`: Tool execution complete
- `conversation_complete`: Turn finished

#### `api_server.py`
**Purpose**: FastAPI web server exposing WebSocket and REST endpoints

**Endpoints**:
- `WS /ws`: WebSocket for real-time chat (recommended)
- `POST /chat`: HTTP endpoint for non-streaming chat
- `GET /tools`: List available MCP tools
- `GET /health`: Health check

**Startup Process**:
1. Load environment variables (ANTHROPIC_API_KEY, MCP_HOST, MCP_PORT)
2. Initialize MCP client and connect to EdgeLake
3. Initialize Claude integration
4. Start FastAPI server

**WebSocket Message Format**:
- Client → Server: `{"message": "user message here"}`
- Server → Client: `{"type": "event_type", "data": {...}}`

**Important**: Uses global state for `mcp_client`, `claude`, and `conversation_history`. In production, should use proper session management.

### Frontend (`frontend/`)

#### `src/components/Chat.tsx`
**Purpose**: Main chat interface component

**State Management**:
- `messages`: Array of user/assistant messages
- `isConnected`: WebSocket connection status
- `isTyping`: Whether Claude is currently responding
- `currentToolCalls`: Tool calls in progress/completed

**WebSocket Events Handled**:
- `message_received`: Acknowledgment from server
- `text_delta`: Streaming text chunks from Claude
- `tool_executing`: Tool is being called
- `tool_result`: Tool execution complete
- `conversation_complete`: Response finished
- `error`: Error occurred

**Features**:
- Real-time message streaming
- Connection status indicator
- Auto-scroll to latest message
- Enter to send (Shift+Enter for newline)
- Tool execution visualization

#### `src/components/ToolExecution.tsx`
**Purpose**: Visualizes tool calls with collapsible details

**Display**:
- Tool name with icon
- Status badge (Running/Complete)
- Expandable details showing:
  - Tool input (JSON formatted)
  - Tool result (JSON or formatted list)

**Styling**: Dark theme with syntax highlighting for JSON data

## Environment Configuration

### Backend `.env`
```bash
ANTHROPIC_API_KEY=sk-ant-...    # Required: Anthropic API key
MCP_HOST=<edgelake-lan-ip>           # EdgeLake MCP server host
MCP_PORT=50051                   # EdgeLake MCP server port (default 50051)
```

### Frontend
WebSocket URL is dynamically constructed:
```typescript
const wsUrl = `ws://${window.location.hostname}:8000/ws`;
```

For different deployment scenarios, update this in `App.tsx`.

## Development Workflow

### Running Locally

**Backend**:
```bash
cd backend
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
cp .env.example .env
# Edit .env with your ANTHROPIC_API_KEY
python api_server.py
```

**Frontend**:
```bash
cd frontend
npm install
npm run dev
```

### Testing MCP Client Independently
```bash
cd backend
python mcp_client.py
```

This will:
1. Connect to MCP server at configured host:port
2. Discover and list available tools
3. Test the `list_databases` tool
4. Print results

### Testing Claude Integration
```bash
cd backend
export ANTHROPIC_API_KEY="your-key"
python llm_integration.py
```

## Current Tool Support

Currently implemented:
- `list_databases`: Returns list of databases available in EdgeLake network

**The MCP client automatically discovers ALL tools** from the EdgeLake MCP server, so additional tools become available automatically when added to EdgeLake's `edge_lake/mcp_server/config/tools.yaml`.

## Adding New Features

### Adding More MCP Tools
Tools are defined in EdgeLake MCP server, not in this client. When new tools are added to `edge_lake/mcp_server/config/tools.yaml`, they automatically become available to Claude through the MCP client's tool discovery mechanism.

### Improving Chat UI
Key areas for enhancement:
1. **Message History Persistence**: Currently in-memory only
2. **Multi-Session Support**: Add session management
3. **Data Visualization**: Add charts/graphs for query results
4. **Export Features**: Download results as CSV/JSON
5. **Query Builder UI**: Visual query construction

### Backend Enhancements
1. **Session Management**: Replace global state with proper sessions
2. **Conversation Persistence**: Store conversations in database
3. **Multi-LLM Support**: Add OpenAI, Google, etc.
4. **Caching**: Cache tool results for repeated queries
5. **Rate Limiting**: Add request throttling

## Common Issues

### WebSocket Connection Fails
- Check backend is running on port 8000
- Check CORS settings in `api_server.py`
- Verify firewall allows WebSocket connections

### MCP Server Connection Fails
- Verify EdgeLake MCP server is running (port 50051)
- Check `MCP_HOST` and `MCP_PORT` in `.env`
- Test with `python mcp_client.py`

### Claude Not Using Tools
- Verify tools are discovered (check `/tools` endpoint)
- Check Anthropic API key is valid
- Review Claude integration logs for tool use events

### Tool Execution Errors
- Check EdgeLake MCP server logs
- Verify tool arguments match schema
- Test tool directly with `mcp_client.py`

## Code Style

- **Backend**: PEP 8, type hints, async/await
- **Frontend**: TypeScript strict mode, React functional components with hooks
- **License**: Mozilla Public License 2.0 (same as EdgeLake)

## Dependencies

### Backend
- `anthropic>=0.40.0`: Claude API client
- `fastapi>=0.115.0`: Web framework
- `httpx>=0.27.0`: Async HTTP client for MCP
- `uvicorn[standard]>=0.32.0`: ASGI server
- `websockets>=13.0`: WebSocket support
- `python-dotenv>=1.0.0`: Environment management

### Frontend
- `react`: UI framework
- `typescript`: Type safety
- `vite`: Build tool and dev server

## Future Enhancements

**Phase 1** (Current MVP):
- ✅ MCP SSE client
- ✅ Claude integration with tool use
- ✅ WebSocket chat interface
- ✅ Single tool (`list_databases`)
- ✅ Tool execution visualization

**Phase 2** (Near-term):
- [ ] All EdgeLake MCP tools (list_tables, get_schema, query, etc.)
- [ ] Conversation history persistence
- [ ] Error recovery and retry logic
- [ ] Connection status with auto-reconnect
- [ ] Loading states and progress indicators

**Phase 3** (Mid-term):
- [ ] Query builder UI
- [ ] Data visualization (charts, graphs)
- [ ] Export functionality (CSV, JSON, SQL)
- [ ] Multi-session support
- [ ] User authentication

**Phase 4** (Long-term):
- [ ] Multi-LLM support (OpenAI, Google, etc.)
- [ ] Tool chaining and workflows
- [ ] Saved queries and templates
- [ ] Collaborative features
- [ ] Mobile responsive design

## Testing

Currently no automated tests. Recommended additions:
- Backend: pytest for API endpoints and MCP client
- Frontend: Vitest + React Testing Library
- E2E: Playwright for full workflow testing

## Related Documentation

- EdgeLake MCP Server: `edge_lake/mcp_server/ARCHITECTURE.md`
- MCP Spec: https://spec.modelcontextprotocol.io/
- Anthropic Tool Use: https://docs.anthropic.com/en/docs/build-with-claude/tool-use
- FastAPI: https://fastapi.tiangolo.com/
- React: https://react.dev/

## Questions for Future Sessions

When returning to this project, consider:
1. Should conversation history be stored in SQLite, PostgreSQL, or elsewhere?
2. How to handle authentication/authorization for multi-user deployment?
3. Should this be deployable as a Docker container?
4. What's the strategy for handling long-running queries (>30s)?
5. How to implement proper error boundaries in React?
6. Should we add a query history sidebar?
7. What metrics/analytics should be tracked?

# EdgeLake MCP Client

A full-stack application that connects Claude (Anthropic AI) to EdgeLake via the MCP (Model Context Protocol) SSE server.

## Architecture

```
React Frontend (Browser)
         ↓ WebSocket
FastAPI Backend (Python)
         ↓ MCP Protocol
EdgeLake MCP SSE Server
         ↓ EdgeLake Commands
EdgeLake Network
```

## Features

- **Chat Interface**: Natural language interface to query EdgeLake data
- **Anthropic Claude Integration**: Powered by Claude with tool use capabilities
- **MCP SSE Client**: Connects to EdgeLake MCP server via Server-Sent Events
- **Tool Execution Visualization**: See when Claude calls EdgeLake tools and view results
- **Real-time Streaming**: WebSocket-based streaming responses from Claude

## Setup

### Prerequisites

- Python 3.11+
- Node.js 18+
- Anthropic API key
- EdgeLake MCP server running (port 50051)

### Backend Setup

```bash
cd backend

# Create virtual environment
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r requirements.txt

# Configure environment
cp .env.example .env
# Edit .env and add your ANTHROPIC_API_KEY
```

### Frontend Setup

```bash
cd frontend

# Install dependencies
npm install
```

## Running the Application

### Start Backend

```bash
cd backend
source venv/bin/activate
python api_server.py
```

Backend will start on http://localhost:8000

### Start Frontend

```bash
cd frontend
npm run dev
```

Frontend will start on http://localhost:5173

## Usage

1. Open your browser to http://localhost:5173
2. Once connected, ask questions like:
   - "What databases are available?"
   - "List the tables in the database"
   - "Show me the schema for table X"
   - "Query the sensor data from the last hour"

Claude will automatically use the appropriate EdgeLake tools to answer your questions.

## Project Structure

```
mcp-client/
├── backend/
│   ├── mcp_client.py          # MCP SSE client library
│   ├── llm_integration.py     # Anthropic Claude integration
│   ├── api_server.py          # FastAPI server with WebSocket
│   ├── requirements.txt       # Python dependencies
│   └── .env.example          # Environment template
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── Chat.tsx       # Chat interface
│   │   │   └── ToolExecution.tsx  # Tool call visualization
│   │   ├── App.tsx           # Main app component
│   │   └── main.tsx
│   ├── package.json
│   └── vite.config.ts
└── README.md
```

## Development

### Backend Testing

Test the MCP client directly:

```bash
cd backend
python mcp_client.py
```

Test the Claude integration:

```bash
cd backend
export ANTHROPIC_API_KEY="your-key-here"
python llm_integration.py
```

### Frontend Development

The frontend uses Vite for hot module reloading. Changes will automatically refresh in the browser.

## Environment Variables

### Backend (.env)

```bash
ANTHROPIC_API_KEY=sk-ant-...    # Required: Your Anthropic API key
MCP_HOST=<edgelake-lan-ip>           # EdgeLake MCP server host
MCP_PORT=50051                   # EdgeLake MCP server port
```

## API Endpoints

### REST Endpoints

- `GET /` - Health check
- `GET /tools` - List available MCP tools
- `GET /health` - Health status
- `POST /chat` - Non-streaming chat endpoint

### WebSocket

- `WS /ws` - Real-time chat with streaming responses

## License

Mozilla Public License 2.0

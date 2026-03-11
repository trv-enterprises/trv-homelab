import React, { useState, useEffect, useRef } from 'react';
import ToolExecution from './ToolExecution';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  toolCalls?: ToolCall[];
}

interface ToolCall {
  name: string;
  input: any;
  result: any;
}

interface ChatProps {
  wsUrl: string;
}

const Chat: React.FC<ChatProps> = ({ wsUrl }) => {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [isConnected, setIsConnected] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const [currentToolCalls, setCurrentToolCalls] = useState<ToolCall[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const currentMessageRef = useRef<string>('');

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  useEffect(() => {
    // Connect to WebSocket
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('WebSocket connected');
      setIsConnected(true);
    };

    ws.onclose = () => {
      console.log('WebSocket disconnected');
      setIsConnected(false);
    };

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      handleWebSocketMessage(data);
    };

    return () => {
      ws.close();
    };
  }, [wsUrl]);

  const handleWebSocketMessage = (data: any) => {
    const { type, data: payload } = data;

    switch (type) {
      case 'message_received':
        setIsTyping(true);
        currentMessageRef.current = '';
        setCurrentToolCalls([]);
        break;

      case 'text_delta':
        currentMessageRef.current += payload.text;
        // Update the last message in real-time
        setMessages(prev => {
          const newMessages = [...prev];
          if (newMessages.length > 0 && newMessages[newMessages.length - 1].role === 'assistant') {
            newMessages[newMessages.length - 1].content = currentMessageRef.current;
          } else {
            newMessages.push({ role: 'assistant', content: currentMessageRef.current });
          }
          return newMessages;
        });
        break;

      case 'tool_executing':
        console.log(`Executing tool: ${payload.tool_name}`, payload.tool_input);
        setCurrentToolCalls(prev => [
          ...prev,
          {
            name: payload.tool_name,
            input: payload.tool_input,
            result: null
          }
        ]);
        break;

      case 'tool_result':
        console.log(`Tool result: ${payload.tool_name}`, payload.result);
        setCurrentToolCalls(prev => {
          const updated = [...prev];
          const index = updated.findIndex(tc => tc.name === payload.tool_name && tc.result === null);
          if (index >= 0) {
            updated[index].result = payload.result;
          }
          return updated;
        });
        break;

      case 'conversation_complete':
        setIsTyping(false);
        // Add tool calls to the last assistant message
        if (currentToolCalls.length > 0) {
          setMessages(prev => {
            const newMessages = [...prev];
            if (newMessages.length > 0 && newMessages[newMessages.length - 1].role === 'assistant') {
              newMessages[newMessages.length - 1].toolCalls = currentToolCalls;
            }
            return newMessages;
          });
        }
        break;

      case 'error':
        console.error('Server error:', payload.error);
        setMessages(prev => [
          ...prev,
          { role: 'assistant', content: `Error: ${payload.error}` }
        ]);
        setIsTyping(false);
        break;
    }
  };

  const sendMessage = () => {
    if (!input.trim() || !isConnected) return;

    const userMessage: Message = { role: 'user', content: input };
    setMessages(prev => [...prev, userMessage]);

    wsRef.current?.send(JSON.stringify({ message: input }));
    setInput('');
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <div style={styles.container}>
      <div style={styles.header}>
        <h2>EdgeLake Chat</h2>
        <div style={{
          ...styles.status,
          backgroundColor: isConnected ? '#4caf50' : '#f44336'
        }}>
          {isConnected ? 'Connected' : 'Disconnected'}
        </div>
      </div>

      <div style={styles.messagesContainer}>
        {messages.map((message, index) => (
          <div key={index} style={{
            ...styles.message,
            alignSelf: message.role === 'user' ? 'flex-end' : 'flex-start',
            backgroundColor: message.role === 'user' ? '#2196F3' : '#424242',
          }}>
            <div style={styles.messageContent}>
              {message.content}
            </div>
            {message.toolCalls && message.toolCalls.length > 0 && (
              <div style={styles.toolCallsContainer}>
                {message.toolCalls.map((toolCall, tcIndex) => (
                  <ToolExecution key={tcIndex} toolCall={toolCall} />
                ))}
              </div>
            )}
          </div>
        ))}
        {isTyping && (
          <div style={{ ...styles.message, backgroundColor: '#424242' }}>
            <div style={styles.typing}>Thinking...</div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      <div style={styles.inputContainer}>
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyPress={handleKeyPress}
          placeholder="Ask about your EdgeLake data..."
          style={styles.textarea}
          disabled={!isConnected}
        />
        <button
          onClick={sendMessage}
          disabled={!isConnected || !input.trim()}
          style={{
            ...styles.sendButton,
            opacity: (!isConnected || !input.trim()) ? 0.5 : 1
          }}
        >
          Send
        </button>
      </div>
    </div>
  );
};

const styles = {
  container: {
    display: 'flex',
    flexDirection: 'column' as const,
    height: '100vh',
    maxWidth: '1200px',
    margin: '0 auto',
    backgroundColor: '#1e1e1e',
    color: '#ffffff',
  },
  header: {
    padding: '20px',
    borderBottom: '1px solid #333',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  status: {
    padding: '8px 16px',
    borderRadius: '20px',
    fontSize: '14px',
    fontWeight: 'bold' as const,
  },
  messagesContainer: {
    flex: 1,
    overflowY: 'auto' as const,
    padding: '20px',
    display: 'flex',
    flexDirection: 'column' as const,
    gap: '12px',
  },
  message: {
    padding: '12px 16px',
    borderRadius: '12px',
    maxWidth: '70%',
    wordWrap: 'break-word' as const,
  },
  messageContent: {
    whiteSpace: 'pre-wrap' as const,
  },
  toolCallsContainer: {
    marginTop: '12px',
    borderTop: '1px solid rgba(255,255,255,0.1)',
    paddingTop: '12px',
  },
  typing: {
    fontStyle: 'italic' as const,
    opacity: 0.7,
  },
  inputContainer: {
    padding: '20px',
    borderTop: '1px solid #333',
    display: 'flex',
    gap: '12px',
  },
  textarea: {
    flex: 1,
    padding: '12px',
    borderRadius: '8px',
    border: '1px solid #444',
    backgroundColor: '#2a2a2a',
    color: '#ffffff',
    fontSize: '16px',
    resize: 'none' as const,
    minHeight: '60px',
    fontFamily: 'inherit',
  },
  sendButton: {
    padding: '12px 24px',
    borderRadius: '8px',
    border: 'none',
    backgroundColor: '#2196F3',
    color: 'white',
    fontSize: '16px',
    fontWeight: 'bold' as const,
    cursor: 'pointer',
  },
};

export default Chat;

import React, { useState } from 'react';

interface ToolCall {
  name: string;
  input: any;
  result: any;
}

interface ToolExecutionProps {
  toolCall: ToolCall;
}

const ToolExecution: React.FC<ToolExecutionProps> = ({ toolCall }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  const formatJSON = (obj: any) => {
    return JSON.stringify(obj, null, 2);
  };

  const formatResult = (result: any) => {
    if (Array.isArray(result)) {
      return (
        <div style={styles.resultList}>
          {result.map((item, index) => (
            <div key={index} style={styles.resultItem}>
              {typeof item === 'object' ? formatJSON(item) : String(item)}
            </div>
          ))}
        </div>
      );
    } else if (typeof result === 'object') {
      return <pre style={styles.json}>{formatJSON(result)}</pre>;
    } else {
      return <div>{String(result)}</div>;
    }
  };

  return (
    <div style={styles.container}>
      <div
        style={styles.header}
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <div style={styles.toolName}>
          <span style={styles.icon}>🔧</span>
          {toolCall.name}
        </div>
        <div style={styles.status}>
          {toolCall.result !== null ? (
            <span style={{ ...styles.badge, backgroundColor: '#4caf50' }}>
              ✓ Complete
            </span>
          ) : (
            <span style={{ ...styles.badge, backgroundColor: '#ff9800' }}>
              ⋯ Running
            </span>
          )}
        </div>
        <div style={styles.expandIcon}>
          {isExpanded ? '▼' : '▶'}
        </div>
      </div>

      {isExpanded && (
        <div style={styles.details}>
          {Object.keys(toolCall.input).length > 0 && (
            <div style={styles.section}>
              <div style={styles.sectionTitle}>Input:</div>
              <pre style={styles.json}>{formatJSON(toolCall.input)}</pre>
            </div>
          )}

          {toolCall.result !== null && (
            <div style={styles.section}>
              <div style={styles.sectionTitle}>Result:</div>
              {formatResult(toolCall.result)}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

const styles = {
  container: {
    backgroundColor: '#333',
    borderRadius: '8px',
    marginTop: '8px',
    overflow: 'hidden',
  },
  header: {
    padding: '12px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    cursor: 'pointer',
    userSelect: 'none' as const,
    transition: 'background-color 0.2s',
  },
  toolName: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontWeight: 'bold' as const,
    flex: 1,
  },
  icon: {
    fontSize: '18px',
  },
  status: {
    marginRight: '12px',
  },
  badge: {
    padding: '4px 12px',
    borderRadius: '12px',
    fontSize: '12px',
    fontWeight: 'bold' as const,
  },
  expandIcon: {
    fontSize: '12px',
    opacity: 0.6,
  },
  details: {
    padding: '12px',
    borderTop: '1px solid rgba(255,255,255,0.1)',
    backgroundColor: '#2a2a2a',
  },
  section: {
    marginBottom: '12px',
  },
  sectionTitle: {
    fontSize: '12px',
    textTransform: 'uppercase' as const,
    opacity: 0.6,
    marginBottom: '8px',
    fontWeight: 'bold' as const,
  },
  json: {
    backgroundColor: '#1e1e1e',
    padding: '12px',
    borderRadius: '4px',
    overflow: 'auto',
    fontSize: '13px',
    fontFamily: 'monospace',
    margin: 0,
  },
  resultList: {
    display: 'flex',
    flexDirection: 'column' as const,
    gap: '8px',
  },
  resultItem: {
    backgroundColor: '#1e1e1e',
    padding: '8px 12px',
    borderRadius: '4px',
    fontSize: '13px',
    fontFamily: 'monospace',
    borderLeft: '3px solid #2196F3',
  },
};

export default ToolExecution;

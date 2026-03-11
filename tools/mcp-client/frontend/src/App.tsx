import Chat from './components/Chat';
import './App.css';

function App() {
  // WebSocket URL - adjust if your backend is on a different host/port
  const wsUrl = `ws://${window.location.hostname}:8000/ws`;

  return (
    <div className="App">
      <Chat wsUrl={wsUrl} />
    </div>
  );
}

export default App;

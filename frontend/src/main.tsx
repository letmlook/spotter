import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import { DeviceProvider } from './state/DeviceContext';
import './styles.css';

// The AntD ConfigProvider is mounted inside <App> so it can react to
// the current theme from ThemeContext. We only need the providers
// that don't depend on theme here.
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <DeviceProvider>
      <App />
    </DeviceProvider>
  </React.StrictMode>,
);
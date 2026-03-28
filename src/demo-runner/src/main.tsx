import React from 'react';
import ReactDOM from 'react-dom/client';
import { DemoProvider } from './contexts/DemoContext';
import App from './App';

const globalStyles = document.createElement('style');
globalStyles.textContent = `
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #12122a; color: #d0d0e0; }
  .json-string { color: #ce9178; }
  .json-number { color: #b5cea8; }
  .json-boolean { color: #569cd6; }
  .json-null { color: #666; font-style: italic; }
  .json-key { color: #9cdcfe; }
`;
document.head.appendChild(globalStyles);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <DemoProvider>
      <App />
    </DemoProvider>
  </React.StrictMode>,
);

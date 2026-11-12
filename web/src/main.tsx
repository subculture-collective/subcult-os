import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import './styles.css';
import { AppearanceControl } from './ui/AppearanceControl';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
<div role="region" aria-label="Display settings" className="flex justify-end bg-surface-canvas px-4 py-2 sm:px-6"><AppearanceControl /></div>
<App />
  </StrictMode>,
);

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { BrowserRouter } from 'react-router-dom'
import { ThemeProvider } from './contexts/ThemeContext.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <BrowserRouter basename={window.location.pathname.startsWith('/server') ? '/server' : '/'}>
        <App />
      </BrowserRouter>
    </ThemeProvider>
  </StrictMode>,
)

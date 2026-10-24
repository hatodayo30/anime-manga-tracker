import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { WorkModalProvider } from './components/WorkModalContext.tsx'
import { AuthProvider } from './context/AuthContext.tsx'
import { TitleLangProvider } from './context/TitleLangContext.tsx'
import './index.css'
import './styles.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <TitleLangProvider>
          <WorkModalProvider>
            <App />
          </WorkModalProvider>
        </TitleLangProvider>
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
)

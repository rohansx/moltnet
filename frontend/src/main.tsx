import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import './styles/system.css';
import './styles/pages.css';
import { AuthProvider, useAuth } from './lib/auth';
import { Login } from './pages/Login';
import { Dashboard } from './pages/Dashboard';
import { Landing } from './pages/Landing';
import { Explorer } from './pages/Explorer';
import { Profile } from './pages/Profile';
import { Register } from './pages/Register';
import { Graph } from './pages/Graph';
import { Design } from './pages/Design';
import type { ReactNode } from 'react';

function RequireAuth({ children }: { children: ReactNode }) {
  const { owner, loading } = useAuth();
  if (loading) {
    return (
      <div style={{ padding: 60, textAlign: 'center', color: 'var(--ink-4)', fontSize: 12 }}>
        loading session…
      </div>
    );
  }
  if (!owner) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* public — an open registry: anyone can browse and verify */}
          <Route path="/" element={<Landing />} />
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route path="/explorer" element={<Explorer />} />
          <Route path="/profile/:did" element={<Profile />} />
          <Route path="/graph" element={<Graph />} />
          <Route path="/design" element={<Design />} />

          {/* private — your agents + credential management */}
          <Route
            path="/dashboard"
            element={
              <RequireAuth>
                <Dashboard />
              </RequireAuth>
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
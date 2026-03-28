import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './contexts/AuthContext';
import { TradingProvider } from './contexts/MarketContext';
import { Login } from './pages/Login';
import { Trading } from './pages/Trading';
import { NotFound } from './pages/NotFound';
import './styles/global.css';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { state } = useAuth();

  if (state.status === 'idle' || state.status === 'loading') {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}>
        Loading...
      </div>
    );
  }

  if (state.status !== 'authenticated') {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

function AppRoutes() {
  const { state } = useAuth();

  return (
    <Routes>
      <Route
        path="/login"
        element={state.status === 'authenticated' ? <Navigate to="/trade" replace /> : <Login />}
      />
      <Route
        path="/trade"
        element={
          <ProtectedRoute>
            <TradingProvider>
              <Trading />
            </TradingProvider>
          </ProtectedRoute>
        }
      />
      <Route
        path="/trade/:instrumentId"
        element={
          <ProtectedRoute>
            <TradingProvider>
              <Trading />
            </TradingProvider>
          </ProtectedRoute>
        }
      />
      <Route path="/" element={<Navigate to="/trade" replace />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  );
}

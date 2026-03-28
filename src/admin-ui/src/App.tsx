import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import { LoginPage } from './pages/Login';
import { DashboardLayout } from './pages/DashboardLayout';
import { DashboardHome } from './pages/DashboardHome';
import { SystemMonitoring } from './pages/SystemMonitoring';
import { ParticipantsPage } from './pages/Participants';
import { MarginCallsPage } from './pages/MarginCalls';
import { SettlementStatusPage } from './pages/SettlementStatus';
import { CircuitBreakersPage } from './pages/CircuitBreakers';
import { WarehouseOverviewPage } from './pages/WarehouseOverview';
import { ComplianceAlertsPage } from './pages/ComplianceAlerts';
import { AuditLogPage } from './pages/AuditLog';

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/dashboard" element={<DashboardLayout />}>
            <Route index element={<DashboardHome />} />
            <Route path="monitoring" element={<SystemMonitoring />} />
            <Route path="participants" element={<ParticipantsPage />} />
            <Route path="margin" element={<MarginCallsPage />} />
            <Route path="settlement" element={<SettlementStatusPage />} />
            <Route path="circuit-breakers" element={<CircuitBreakersPage />} />
            <Route path="warehouse" element={<WarehouseOverviewPage />} />
            <Route path="compliance" element={<ComplianceAlertsPage />} />
            <Route path="audit" element={<AuditLogPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

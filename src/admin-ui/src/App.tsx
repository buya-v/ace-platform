import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import { RoleGuard } from './components/RoleGuard';
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
import { OrderBookPage } from './pages/OrderBook';
import { PositionsPage } from './pages/Positions';
import { RiskOverviewPage } from './pages/RiskOverview';

const ADMIN_ROLES = ['admin', 'exchange_admin'];

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/dashboard" element={<DashboardLayout />}>
            <Route index element={<DashboardHome />} />
            {/* Admin-only routes */}
            <Route element={<RoleGuard allowedRoles={ADMIN_ROLES} />}>
              <Route path="monitoring" element={<SystemMonitoring />} />
              <Route path="orderbook" element={<OrderBookPage />} />
              <Route path="positions" element={<PositionsPage />} />
              <Route path="risk" element={<RiskOverviewPage />} />
              <Route path="margin" element={<MarginCallsPage />} />
              <Route path="settlement" element={<SettlementStatusPage />} />
              <Route path="circuit-breakers" element={<CircuitBreakersPage />} />
              <Route path="warehouse" element={<WarehouseOverviewPage />} />
            </Route>
            {/* Admin + compliance routes */}
            <Route path="participants" element={<ParticipantsPage />} />
            <Route path="compliance" element={<ComplianceAlertsPage />} />
            <Route path="audit" element={<AuditLogPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

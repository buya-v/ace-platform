import React from 'react';
import { Navigate, Outlet } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

interface RoleGuardProps {
  allowedRoles: string[];
  children?: React.ReactNode;
}

export function RoleGuard({ allowedRoles, children }: RoleGuardProps) {
  const { state } = useAuth();
  const userRoles = state.user?.roles ?? [];

  const hasAccess = userRoles.some(role => allowedRoles.includes(role));

  if (!hasAccess) {
    return <Navigate to="/dashboard" replace />;
  }

  return children ? <>{children}</> : <Outlet />;
}

import React from 'react';
import { NavLink } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { hasAdminAccess } from '../types';
import styles from './Sidebar.module.css';

interface NavItem {
  label: string;
  path: string;
  adminOnly: boolean;
}

const operationsNav: NavItem[] = [
  { label: 'System Health', path: '/dashboard/monitoring', adminOnly: true },
  { label: 'Margin Calls', path: '/dashboard/margin', adminOnly: true },
  { label: 'Settlement', path: '/dashboard/settlement', adminOnly: true },
  { label: 'Circuit Breakers', path: '/dashboard/circuit-breakers', adminOnly: true },
  { label: 'Warehouse', path: '/dashboard/warehouse', adminOnly: true },
];

const complianceNav: NavItem[] = [
  { label: 'Participants', path: '/dashboard/participants', adminOnly: false },
  { label: 'Compliance Alerts', path: '/dashboard/compliance', adminOnly: false },
  { label: 'Audit Log', path: '/dashboard/audit', adminOnly: false },
];

export function Sidebar() {
  const { state, logout } = useAuth();
  const isAdmin = hasAdminAccess(state.user?.roles ?? []);

  return (
    <nav className={styles.sidebar} data-testid="sidebar">
      <div className={styles.logo}>ACE Admin</div>

      <NavLink to="/dashboard" end className={({ isActive }) => isActive ? styles.activeLink : styles.link}>
        Overview
      </NavLink>

      {isAdmin && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>Operations</div>
          {operationsNav.map(item => (
            <NavLink
              key={item.path}
              to={item.path}
              className={({ isActive }) => isActive ? styles.activeLink : styles.link}
            >
              {item.label}
            </NavLink>
          ))}
        </div>
      )}

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Compliance</div>
        {complianceNav.map(item => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) => isActive ? styles.activeLink : styles.link}
          >
            {item.label}
          </NavLink>
        ))}
      </div>

      <div className={styles.userSection}>
        <div className={styles.userName}>{state.user?.name ?? 'User'}</div>
        <button onClick={logout} className={styles.logoutBtn}>Logout</button>
      </div>
    </nav>
  );
}

export { operationsNav, complianceNav };
export type { NavItem };

import React, { useState } from 'react';
import { NavLink } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { hasAdminAccess } from '../types';
import {
  DashboardIcon,
  HeartbeatIcon,
  DollarIcon,
  ExchangeIcon,
  ShieldIcon,
  BoxIcon,
  UsersIcon,
  FlagIcon,
  ClipboardIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  BookIcon,
  AlertIcon,
  ChartBarIcon,
} from './icons';
import styles from './Sidebar.module.css';

interface NavItem {
  label: string;
  path: string;
  adminOnly: boolean;
  icon: React.ComponentType<{ size?: number; className?: string }>;
}

const operationsNav: NavItem[] = [
  { label: 'System Health', path: '/dashboard/monitoring', adminOnly: true, icon: HeartbeatIcon },
  { label: 'Order Book', path: '/dashboard/orderbook', adminOnly: true, icon: BookIcon },
  { label: 'Positions', path: '/dashboard/positions', adminOnly: true, icon: ChartBarIcon },
  { label: 'Risk Overview', path: '/dashboard/risk', adminOnly: true, icon: AlertIcon },
  { label: 'Margin Calls', path: '/dashboard/margin', adminOnly: true, icon: DollarIcon },
  { label: 'Settlement', path: '/dashboard/settlement', adminOnly: true, icon: ExchangeIcon },
  { label: 'Circuit Breakers', path: '/dashboard/circuit-breakers', adminOnly: true, icon: ShieldIcon },
  { label: 'Warehouse', path: '/dashboard/warehouse', adminOnly: true, icon: BoxIcon },
];

const complianceNav: NavItem[] = [
  { label: 'Participants', path: '/dashboard/participants', adminOnly: false, icon: UsersIcon },
  { label: 'Compliance Alerts', path: '/dashboard/compliance', adminOnly: false, icon: FlagIcon },
  { label: 'Audit Log', path: '/dashboard/audit', adminOnly: false, icon: ClipboardIcon },
];

export type SystemStatus = 'healthy' | 'degraded' | 'unhealthy' | 'unknown';

interface SidebarProps {
  systemStatus?: SystemStatus;
}

export function Sidebar({ systemStatus = 'unknown' }: SidebarProps) {
  const { state, logout } = useAuth();
  const isAdmin = hasAdminAccess(state.user?.roles ?? []);
  const [opsOpen, setOpsOpen] = useState(true);
  const [compOpen, setCompOpen] = useState(true);

  const statusColorMap: Record<SystemStatus, string> = {
    healthy: 'var(--accent-green)',
    degraded: 'var(--accent-yellow)',
    unhealthy: 'var(--accent-red)',
    unknown: 'var(--text-muted)',
  };

  return (
    <nav className={styles.sidebar} data-testid="sidebar">
      <div className={styles.logo}>
        <span
          className={styles.statusDot}
          style={{ background: statusColorMap[systemStatus] }}
        />
        ACE Admin
      </div>

      <NavLink to="/dashboard" end className={({ isActive }) => isActive ? styles.activeLink : styles.link}>
        <DashboardIcon size={16} className={styles.navIcon} />
        Overview
      </NavLink>

      {isAdmin && (
        <div className={styles.section}>
          <button
            className={styles.sectionToggle}
            onClick={() => setOpsOpen(o => !o)}
            aria-expanded={opsOpen}
          >
            {opsOpen ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
            <span>Operations</span>
          </button>
          {opsOpen && operationsNav.map(item => (
            <NavLink
              key={item.path}
              to={item.path}
              className={({ isActive }) => isActive ? styles.activeLink : styles.link}
            >
              <item.icon size={16} className={styles.navIcon} />
              {item.label}
            </NavLink>
          ))}
        </div>
      )}

      <div className={styles.section}>
        <button
          className={styles.sectionToggle}
          onClick={() => setCompOpen(o => !o)}
          aria-expanded={compOpen}
        >
          {compOpen ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
          <span>Compliance</span>
        </button>
        {compOpen && complianceNav.map(item => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) => isActive ? styles.activeLink : styles.link}
          >
            <item.icon size={16} className={styles.navIcon} />
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

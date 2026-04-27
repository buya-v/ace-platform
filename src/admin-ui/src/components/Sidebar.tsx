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
  EyeIcon,
  DollarIcon as FeeIcon,
  FileTextIcon,
  TicketIcon,
} from './icons';
import styles from './Sidebar.module.css';

interface NavItem {
  label: string;
  path: string;
  adminOnly: boolean;
  icon: React.ComponentType<{ size?: number; className?: string }>;
}

// Securities-first navigation — these are the primary pages for MSE equities demo
const securitiesNav: NavItem[] = [
  { label: 'Instruments', path: '/dashboard/securities', adminOnly: true, icon: ChartBarIcon },
  { label: 'Orders', path: '/dashboard/securities-orders', adminOnly: true, icon: BookIcon },
  { label: 'Positions', path: '/dashboard/securities-positions', adminOnly: true, icon: DollarIcon },
  { label: 'Surveillance', path: '/dashboard/surveillance', adminOnly: true, icon: EyeIcon },
  { label: 'Market Phase', path: '/dashboard/market-phase', adminOnly: true, icon: ShieldIcon },
  { label: 'Circuit Breakers', path: '/dashboard/circuit-breakers', adminOnly: true, icon: ShieldIcon },
  { label: 'Settlement', path: '/dashboard/settlement', adminOnly: true, icon: ExchangeIcon },
  { label: 'Reports', path: '/dashboard/reports', adminOnly: true, icon: FileTextIcon },
];

const operationsNav: NavItem[] = [
  { label: 'Platform', path: '/dashboard/platform', adminOnly: true, icon: ShieldIcon },
  { label: 'System Health', path: '/dashboard/monitoring', adminOnly: true, icon: HeartbeatIcon },
  { label: 'Fee Management', path: '/dashboard/fees', adminOnly: true, icon: FeeIcon },
  { label: 'Tickets', path: '/dashboard/tickets', adminOnly: true, icon: TicketIcon },
  { label: 'Risk Overview', path: '/dashboard/risk', adminOnly: true, icon: AlertIcon },
  { label: 'Margin Calls', path: '/dashboard/margin', adminOnly: true, icon: DollarIcon },
  { label: 'Commodity Book', path: '/dashboard/orderbook', adminOnly: true, icon: BookIcon },
  { label: 'Commodity Positions', path: '/dashboard/positions', adminOnly: true, icon: ChartBarIcon },
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
  const [secOpen, setSecOpen] = useState(true);
  const [opsOpen, setOpsOpen] = useState(false);
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
        <span className={styles.logoText}>GarudaX Admin</span>
      </div>

      <NavLink to="/dashboard" end className={({ isActive }) => isActive ? styles.activeLink : styles.link}>
        <DashboardIcon size={16} className={styles.navIcon} />
        <span className={styles.navLabel}>Overview</span>
      </NavLink>

      {isAdmin && (
        <>
          <div className={styles.section}>
            <button
              className={styles.sectionToggle}
              onClick={() => setSecOpen(o => !o)}
              aria-expanded={secOpen}
            >
              {secOpen ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
              <span className={styles.sectionLabel}>Securities Exchange</span>
            </button>
            {secOpen && securitiesNav.map(item => (
              <NavLink
                key={item.path}
                to={item.path}
                className={({ isActive }) => isActive ? styles.activeLink : styles.link}
              >
                <item.icon size={16} className={styles.navIcon} />
                <span className={styles.navLabel}>{item.label}</span>
              </NavLink>
            ))}
          </div>
          <div className={styles.section}>
            <button
              className={styles.sectionToggle}
              onClick={() => setOpsOpen(o => !o)}
              aria-expanded={opsOpen}
            >
              {opsOpen ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
              <span className={styles.sectionLabel}>Operations</span>
            </button>
            {opsOpen && operationsNav.map(item => (
              <NavLink
                key={item.path}
                to={item.path}
                className={({ isActive }) => isActive ? styles.activeLink : styles.link}
              >
                <item.icon size={16} className={styles.navIcon} />
                <span className={styles.navLabel}>{item.label}</span>
              </NavLink>
            ))}
          </div>
        </>
      )}

      <div className={styles.section}>
        <button
          className={styles.sectionToggle}
          onClick={() => setCompOpen(o => !o)}
          aria-expanded={compOpen}
        >
          {compOpen ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
          <span className={styles.sectionLabel}>Compliance</span>
        </button>
        {compOpen && complianceNav.map(item => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) => isActive ? styles.activeLink : styles.link}
          >
            <item.icon size={16} className={styles.navIcon} />
            <span className={styles.navLabel}>{item.label}</span>
          </NavLink>
        ))}
      </div>

      <div className={styles.userSection}>
        <div className={styles.userName}>{state.user?.name ?? 'User'}</div>
        <button onClick={logout} className={styles.logoutBtn}>
          <span className={styles.logoutLabel}>Logout</span>
        </button>
      </div>
    </nav>
  );
}

export { securitiesNav, operationsNav, complianceNav };
export type { NavItem };

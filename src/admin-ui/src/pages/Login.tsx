import React, { useState, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { hasComplianceAccess } from '../types';
import styles from './Login.module.css';

export function LoginPage() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const { state, login } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await login(email, password);
    } catch {
      return;
    }
  };

  // Redirect after successful login
  React.useEffect(() => {
    if (state.isAuthenticated && state.user) {
      if (!hasComplianceAccess(state.user.roles)) {
        return; // Will show unauthorized message
      }
      navigate('/dashboard', { replace: true });
    }
  }, [state.isAuthenticated, state.user, navigate]);

  if (state.isAuthenticated && state.user && !hasComplianceAccess(state.user.roles)) {
    return (
      <div className={styles.container}>
        <div className={styles.card}>
          <h1>Unauthorized</h1>
          <p>Your account does not have admin or compliance access.</p>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <form className={styles.card} onSubmit={handleSubmit}>
        <h1 className={styles.title}>GarudaX Admin</h1>
        <p className={styles.subtitle}>Sign in to the admin dashboard</p>

        {state.error && <div className={styles.error} role="alert">{state.error}</div>}

        <label className={styles.label}>
          Email
          <input
            type="email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            className={styles.input}
            required
            autoFocus
          />
        </label>

        <label className={styles.label}>
          Password
          <input
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            className={styles.input}
            required
          />
        </label>

        <button type="submit" className={styles.button} disabled={state.isLoading}>
          {state.isLoading ? 'Signing in...' : 'Sign In'}
        </button>
      </form>
    </div>
  );
}

import React, { useState } from 'react';
import { useAuth } from '../contexts/AuthContext';
import styles from './Login.module.css';

export const Login: React.FC = () => {
  const { state, login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await login(email, password);
    } catch {
      // Error shown via state.error
    }
  };

  return (
    <div className={styles.page}>
      <form className={styles.form} onSubmit={handleSubmit}>
        <h1 className={styles.title}>ACE Trading Platform</h1>
        <p className={styles.subtitle}>Agriculture Commodity Exchange</p>

        {state.error && <div className={styles.error}>{state.error}</div>}

        <div className={styles.field}>
          <label htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
          />
        </div>

        <button
          type="submit"
          className={styles.submit}
          disabled={state.status === 'loading'}
        >
          {state.status === 'loading' ? 'Logging in...' : 'Login'}
        </button>
      </form>
    </div>
  );
};

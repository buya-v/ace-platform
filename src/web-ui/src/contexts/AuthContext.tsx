import React, { createContext, useContext, useReducer, useEffect, useCallback } from 'react';
import { login as apiLogin, logout as apiLogout, silentRefresh } from '../services/api';

interface User {
  id: string;
  email: string;
  displayName: string;
  roles: string[];
  participantId: string;
}

interface AuthState {
  status: 'idle' | 'loading' | 'authenticated' | 'unauthenticated';
  user: User | null;
  error: string | null;
}

type AuthAction =
  | { type: 'LOGIN_START' }
  | { type: 'LOGIN_SUCCESS'; user: User }
  | { type: 'LOGIN_FAILURE'; error: string }
  | { type: 'LOGOUT' }
  | { type: 'TOKEN_REFRESHED' }
  | { type: 'SESSION_EXPIRED' };

export function authReducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'LOGIN_START':
      return { ...state, status: 'loading', error: null };
    case 'LOGIN_SUCCESS':
      return { status: 'authenticated', user: action.user, error: null };
    case 'LOGIN_FAILURE':
      return { status: 'unauthenticated', user: null, error: action.error };
    case 'LOGOUT':
    case 'SESSION_EXPIRED':
      return { status: 'unauthenticated', user: null, error: null };
    case 'TOKEN_REFRESHED':
      return state;
    default:
      return state;
  }
}

const initialState: AuthState = {
  status: 'idle',
  user: null,
  error: null,
};

interface AuthContextValue {
  state: AuthState;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(authReducer, initialState);

  useEffect(() => {
    // Skip silent refresh — tokens are in-memory only, so there's no session
    // to restore on page load. Go straight to login page.
    dispatch({ type: 'SESSION_EXPIRED' });
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    dispatch({ type: 'LOGIN_START' });
    try {
      const result = await apiLogin(email, password);
      dispatch({ type: 'LOGIN_SUCCESS', user: result.user });
    } catch (err) {
      dispatch({ type: 'LOGIN_FAILURE', error: err instanceof Error ? err.message : 'Login failed' });
      throw err;
    }
  }, []);

  const logout = useCallback(async () => {
    await apiLogout();
    dispatch({ type: 'LOGOUT' });
  }, []);

  return (
    <AuthContext.Provider value={{ state, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

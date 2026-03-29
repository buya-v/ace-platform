import React, { createContext, useContext, useReducer, useCallback, useEffect } from 'react';
import { User } from '../types';
import { login as apiLogin, logout as apiLogout, setAccessToken, setOnUnauthorized } from '../services/api';

interface AuthState {
  token: string | null;
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
}

type AuthAction =
  | { type: 'LOGIN_START' }
  | { type: 'LOGIN_SUCCESS'; payload: { token: string; user: User } }
  | { type: 'LOGIN_FAILURE'; payload: { error: string } }
  | { type: 'LOGOUT' }
  | { type: 'TOKEN_REFRESHED'; payload: { token: string } };

const initialState: AuthState = {
  token: null,
  user: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

export function authReducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'LOGIN_START':
      return { ...state, isLoading: true, error: null };
    case 'LOGIN_SUCCESS':
      return {
        token: action.payload.token,
        user: action.payload.user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
      };
    case 'LOGIN_FAILURE':
      return {
        ...initialState,
        error: action.payload.error,
      };
    case 'LOGOUT':
      return { ...initialState };
    case 'TOKEN_REFRESHED':
      return { ...state, token: action.payload.token };
    default:
      return state;
  }
}

function decodeJwtPayload(token: string): Record<string, unknown> {
  const parts = token.split('.');
  if (parts.length !== 3) throw new Error('Invalid JWT');
  const payload = atob(parts[1].replace(/-/g, '+').replace(/_/g, '/'));
  return JSON.parse(payload);
}

interface AuthContextValue {
  state: AuthState;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(authReducer, initialState);

  useEffect(() => {
    setOnUnauthorized(() => {
      dispatch({ type: 'LOGOUT' });
      setAccessToken(null);
    });
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    dispatch({ type: 'LOGIN_START' });
    try {
      const response = await apiLogin(email, password);
      const raw = response as Record<string, unknown>;
      const accessToken = (raw.access_token || raw.AccessToken || raw.token) as string;
      if (!accessToken) throw new Error('No access token in response');
      const claims = decodeJwtPayload(accessToken);
      const user: User = {
        id: (claims.sub as string) ?? '',
        email: (claims.email as string) ?? email,
        name: (claims.name as string) ?? email,
        roles: (claims.roles as string[]) ?? [],
        participant_id: (claims.participant_id as string) ?? null,
      };
      setAccessToken(accessToken);
      dispatch({ type: 'LOGIN_SUCCESS', payload: { token: accessToken, user } });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Login failed';
      dispatch({ type: 'LOGIN_FAILURE', payload: { error: message } });
      throw err;
    }
  }, []);

  const logoutFn = useCallback(() => {
    apiLogout().catch(() => {});
    setAccessToken(null);
    dispatch({ type: 'LOGOUT' });
  }, []);

  return (
    <AuthContext.Provider value={{ state, login, logout: logoutFn }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

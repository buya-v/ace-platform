import { describe, it, expect } from 'vitest';
import { authReducer } from '../contexts/AuthContext';

const initialState = {
  token: null,
  user: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

const mockUser = {
  id: 'user-1',
  email: 'admin@garudax.mn',
  name: 'Admin User',
  roles: ['admin'],
  participant_id: null,
};

describe('authReducer', () => {
  it('handles LOGIN_START', () => {
    const state = authReducer(initialState, { type: 'LOGIN_START' });
    expect(state.isLoading).toBe(true);
    expect(state.error).toBeNull();
  });

  it('handles LOGIN_SUCCESS', () => {
    const state = authReducer(initialState, {
      type: 'LOGIN_SUCCESS',
      payload: { token: 'jwt-token', user: mockUser },
    });
    expect(state.isAuthenticated).toBe(true);
    expect(state.token).toBe('jwt-token');
    expect(state.user).toEqual(mockUser);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
  });

  it('handles LOGIN_FAILURE', () => {
    const loadingState = { ...initialState, isLoading: true };
    const state = authReducer(loadingState, {
      type: 'LOGIN_FAILURE',
      payload: { error: 'Invalid credentials' },
    });
    expect(state.isAuthenticated).toBe(false);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBe('Invalid credentials');
    expect(state.token).toBeNull();
    expect(state.user).toBeNull();
  });

  it('handles LOGOUT', () => {
    const authedState = {
      token: 'jwt-token',
      user: mockUser,
      isAuthenticated: true,
      isLoading: false,
      error: null,
    };
    const state = authReducer(authedState, { type: 'LOGOUT' });
    expect(state.isAuthenticated).toBe(false);
    expect(state.token).toBeNull();
    expect(state.user).toBeNull();
  });

  it('handles TOKEN_REFRESHED', () => {
    const authedState = {
      token: 'old-token',
      user: mockUser,
      isAuthenticated: true,
      isLoading: false,
      error: null,
    };
    const state = authReducer(authedState, {
      type: 'TOKEN_REFRESHED',
      payload: { token: 'new-token' },
    });
    expect(state.token).toBe('new-token');
    expect(state.user).toEqual(mockUser);
    expect(state.isAuthenticated).toBe(true);
  });

  it('clears error on LOGIN_START after previous failure', () => {
    const errorState = { ...initialState, error: 'Previous error' };
    const state = authReducer(errorState, { type: 'LOGIN_START' });
    expect(state.error).toBeNull();
    expect(state.isLoading).toBe(true);
  });

  it('returns current state for unknown action', () => {
    const state = authReducer(initialState, { type: 'UNKNOWN' } as never);
    expect(state).toBe(initialState);
  });
});

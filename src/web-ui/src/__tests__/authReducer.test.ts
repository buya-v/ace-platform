import { describe, it, expect } from 'vitest';
import { authReducer } from '../contexts/AuthContext';

const initialState = {
  status: 'idle' as const,
  user: null,
  error: null,
};

const mockUser = {
  id: 'u1',
  email: 'test@example.com',
  displayName: 'Test User',
  roles: ['trader'],
  participantId: 'p1',
};

describe('authReducer', () => {
  it('LOGIN_START sets loading status', () => {
    const result = authReducer(initialState, { type: 'LOGIN_START' });
    expect(result.status).toBe('loading');
    expect(result.error).toBeNull();
  });

  it('LOGIN_SUCCESS sets authenticated with user', () => {
    const result = authReducer(initialState, { type: 'LOGIN_SUCCESS', user: mockUser });
    expect(result.status).toBe('authenticated');
    expect(result.user).toEqual(mockUser);
    expect(result.error).toBeNull();
  });

  it('LOGIN_FAILURE sets unauthenticated with error', () => {
    const result = authReducer(initialState, { type: 'LOGIN_FAILURE', error: 'Bad creds' });
    expect(result.status).toBe('unauthenticated');
    expect(result.user).toBeNull();
    expect(result.error).toBe('Bad creds');
  });

  it('LOGIN_START clears previous error', () => {
    const state = { ...initialState, error: 'old error' };
    const result = authReducer(state, { type: 'LOGIN_START' });
    expect(result.error).toBeNull();
  });

  it('LOGOUT clears user and sets unauthenticated', () => {
    const state = { status: 'authenticated' as const, user: mockUser, error: null };
    const result = authReducer(state, { type: 'LOGOUT' });
    expect(result.status).toBe('unauthenticated');
    expect(result.user).toBeNull();
  });

  it('SESSION_EXPIRED clears user and sets unauthenticated', () => {
    const state = { status: 'authenticated' as const, user: mockUser, error: null };
    const result = authReducer(state, { type: 'SESSION_EXPIRED' });
    expect(result.status).toBe('unauthenticated');
    expect(result.user).toBeNull();
  });

  it('TOKEN_REFRESHED keeps current state', () => {
    const state = { status: 'authenticated' as const, user: mockUser, error: null };
    const result = authReducer(state, { type: 'TOKEN_REFRESHED' });
    expect(result).toEqual(state);
  });
});

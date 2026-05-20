import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { authApi } from '../services/api';
import type { UserProfile, UserPerms } from '../types';

interface AuthContextValue {
  token: string | null;
  user: UserProfile | null;
  perms: UserPerms;
  isLoading: boolean;
  login: (token: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'));
  const [user, setUser] = useState<UserProfile | null>(null);
  const [perms, setPerms] = useState<UserPerms>({ isAdmin: false, isEditor: false });
  const [isLoading, setIsLoading] = useState(true);

  const loadProfile = useCallback(async () => {
    try {
      const [meRes, permsRes] = await Promise.all([authApi.me(), authApi.perms()]);
      setUser(meRes.data);
      setPerms(permsRes.data);
    } catch {
      localStorage.removeItem('token');
      setToken(null);
      setUser(null);
      setPerms({ isAdmin: false, isEditor: false });
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!token) {
      setIsLoading(false);
      return;
    }
    loadProfile();
  }, [token, loadProfile]);

  const login = useCallback(async (newToken: string) => {
    localStorage.setItem('token', newToken);
    setToken(newToken);
    // Profile is loaded by the useEffect above after token changes.
    // But we need to await it here for the redirect to happen after profile is set.
    try {
      const [meRes, permsRes] = await Promise.all([authApi.me(), authApi.perms()]);
      setUser(meRes.data);
      setPerms(permsRes.data);
    } catch {
      // If profile fetch fails right after login, clean up.
      localStorage.removeItem('token');
      setToken(null);
      throw new Error('Failed to load user profile after login');
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } catch {
      // Ignore logout API errors — revoke locally regardless.
    }
    localStorage.removeItem('token');
    setToken(null);
    setUser(null);
    setPerms({ isAdmin: false, isEditor: false });
  }, []);

  return (
    <AuthContext.Provider value={{ token, user, perms, isLoading, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider');
  return ctx;
}

// lib/auth.tsx — session state for the dashboard. The HttpOnly session cookie
// is set by the server on login; the client only needs to know who is signed in
// (via /v1/auth/me) and to call logout. Cookies travel automatically for
// same-origin requests.
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { api, type Agent, type MeResponse } from './api';

interface AuthState {
  owner: string | null;
  agents: Agent[];
  loading: boolean;
  refresh: () => Promise<void>;
  signOut: () => Promise<void>;
}

const Ctx = createContext<AuthState>({
  owner: null,
  agents: [],
  loading: true,
  refresh: async () => {},
  signOut: async () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [owner, setOwner] = useState<string | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);

  async function refresh() {
    try {
      const me: MeResponse = await api.me();
      setOwner(me.owner_did);
      setAgents(me.agents || []);
    } catch {
      setOwner(null);
      setAgents([]);
    } finally {
      setLoading(false);
    }
  }

  async function signOut() {
    try {
      await api.logout();
    } catch {
      /* ignore */
    }
    setOwner(null);
    setAgents([]);
  }

  useEffect(() => {
    refresh();
  }, []);

  return <Ctx.Provider value={{ owner, agents, loading, refresh, signOut }}>{children}</Ctx.Provider>;
}

export const useAuth = () => useContext(Ctx);
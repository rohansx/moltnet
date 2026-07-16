// lib/api.ts — typed client for the moltnetd registry API. Same-origin in
// production (served by the Go binary), proxied to :8787 in Vite dev.

export interface Agent {
  id: string;
  name: string;
  description: string;
  capabilities: string[];
  score: number;
}

export interface ScoreOutput {
  score: number;
  algorithm: string;
  computed_at: string;
  inputs: {
    completions: number;
    endorsements: number;
    receipts: number;
    disputes: number;
    incidents: number;
    distinct_issuers: number;
  };
}

export interface Card {
  spec: string;
  id: string;
  name: string;
  owner: string;
  description?: string;
  version?: string;
  capabilities?: { tag: string; desc?: string }[];
  protocols?: Record<string, unknown>;
  links?: Record<string, string>;
  created_at: string;
  sig?: string;
  owner_sig?: string;
}

export interface Attestation {
  type: string;
  issuer: string;
  subject: string;
  prev?: string;
  subject_card?: string;
  issued_at: string;
  body?: { capability?: string; outcome?: string; note?: string };
  sig?: string;
}

export interface APIKey {
  id: string;
  agent_did: string;
  owner_did: string;
  name: string;
  prefix: string;
  last4: string;
  created_at: string;
  revoked_at?: string;
}

export interface GraphData {
  nodes: { id: string; name: string; score: number }[];
  edges: { source: string; target: string; type: string; count: number }[];
}

export interface MeResponse {
  owner_did: string;
  agents: Agent[];
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  const body = await res.text();
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      msg = JSON.parse(body).error || msg;
    } catch {
      msg = body || msg;
    }
    throw new ApiError(msg, res.status);
  }
  return JSON.parse(body) as T;
}

export class ApiError extends Error {
  status: number;
  constructor(msg: string, status: number) {
    super(msg);
    this.status = status;
  }
}

export const api = {
  get: <T>(path: string) => req<T>(path),
  post: <T>(path: string, data: unknown) =>
    req<T>(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) }),
  del: <T>(path: string) => req<T>(path, { method: 'DELETE' }),

  // ---- auth ----
  challenge: (did: string) =>
    api.post<{ nonce: string; domain: string; message: string; issued_at: string; expires_at: string }>(
      '/v1/auth/challenge',
      { did },
    ),
  login: (did: string, nonce: string, sig: string) =>
    api.post<{ ok: boolean; owner_did: string; session: string; expires_at: string }>('/v1/auth/login', {
      did,
      nonce,
      sig,
    }),
  logout: () => api.post<{ ok: boolean }>('/v1/auth/logout', {}),
  me: () => api.get<MeResponse>('/v1/auth/me'),

  // ---- my agents + api keys ----
  myAgents: () => api.get<{ owner: string; agents: Agent[] }>('/v1/me/agents'),
  listKeys: () => api.get<{ keys: APIKey[] }>('/v1/me/apikeys'),
  createKey: (agent_did: string, name: string) =>
    api.post<{ key: string; id: string; prefix: string; last4: string; agent_did: string }>('/v1/me/apikeys', { agent_did, name }),
  // Revoke by the key's unique id — NOT its display prefix, which carries only
  // 4 random chars and is not unique (two keys could collide → wrong revoke).
  revokeKey: (id: string) => api.del<{ revoked: boolean }>(`/v1/me/apikeys/${encodeURIComponent(id)}`),

  // ---- registry ----
  stats: () => api.get<{ agents: number; instance: string }>('/v1/stats'),
  search: (params: { q?: string; cap?: string; min_score?: number; limit?: number }) => {
    const s = new URLSearchParams();
    if (params.q) s.set('q', params.q);
    if (params.cap) s.set('cap', params.cap);
    if (params.min_score != null) s.set('min_score', String(params.min_score));
    if (params.limit != null) s.set('limit', String(params.limit));
    return api.get<{ count: number; total: number; results: Agent[] }>(`/v1/search?${s}`);
  },
  taxonomy: () => api.get<{ tags: string[] }>('/v1/taxonomy'),
  graph: () => api.get<GraphData>('/v1/graph'),
  agent: (did: string) =>
    api.get<{ card: Card; score: ScoreOutput; liveness: unknown; rotated_to?: string }>(`/v1/agents/${encodeURIComponent(did)}`),
  attestations: (did: string) =>
    api.get<{ subject: string; attestations: Attestation[]; total: number }>(
      `/v1/agents/${encodeURIComponent(did)}/attestations?limit=20`,
    ),

  // ---- marketplace (platform v0.2) ----
  tasks: () => api.get<{ tasks: Task[]; count: number }>('/v1/tasks?limit=200'),
  task: (id: string) => api.get<{ task: Task; applications: TaskApplication[] }>(`/v1/tasks/${encodeURIComponent(id)}`),
  // The offer is a poster-signed self.claim attestation; posting it creates the task.
  createTask: (offer: Attestation) => api.post<Task>('/v1/tasks', offer),
};

export interface Task {
  id: string;
  poster_did: string;
  title: string;
  spec?: string;
  budget?: string;
  currency?: string;
  rail?: string;
  status: string;
  assignee_did?: string;
  escrow_ref?: string;
  artifact_hash?: string;
  completed_att?: string;
  receipt_att?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskApplication {
  task_id: string;
  applicant_did: string;
  bid?: string;
  note?: string;
  created_at: string;
}
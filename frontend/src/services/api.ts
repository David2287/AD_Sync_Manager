import axios from 'axios';
import type {
  Employee,
  PaginatedResponse,
  AuditLog,
  IntegrityReport,
  ValidateResponse,
  ApplyResponse,
  UserProfile,
  UserPerms,
} from '../types';

const api = axios.create({ baseURL: '/api/v1' });

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token');
      if (!window.location.pathname.startsWith('/login')) {
        window.location.href = '/login';
      }
    }
    return Promise.reject(err);
  },
);

// ── Auth ─────────────────────────────────────────────────────────────────────

export const authApi = {
  login: (username: string, password: string) =>
    api.post<{ token: string; username: string; dn: string; expires_at: string }>(
      '/auth/login',
      { username, password },
    ),

  logout: () => api.post('/auth/logout'),

  me: () => api.get<UserProfile>('/me'),

  perms: () => api.get<UserPerms>('/me/perms'),
};

// ── Employees ─────────────────────────────────────────────────────────────────

export interface EmployeeListParams {
  limit?: number;
  offset?: number;
  search?: string;
}

export const employeeApi = {
  list: (params: EmployeeListParams) =>
    api.get<PaginatedResponse<Employee>>('/employees', { params }),

  get: (dn: string) =>
    api.get<Employee>(`/employees/${encodeURIComponent(dn)}`),

  update: (dn: string, data: { telephoneNumber?: string; office?: string }) => {
    const body: Record<string, string> = {};
    if (data.telephoneNumber !== undefined) body.telephoneNumber = data.telephoneNumber;
    if (data.office !== undefined) body.physicalDeliveryOfficeName = data.office;
    return api.put<Employee>(`/employees/${encodeURIComponent(dn)}`, body);
  },
};

// ── Markdown ──────────────────────────────────────────────────────────────────

export const markdownApi = {
  validate: (markdown: string) =>
    api.post<ValidateResponse>('/markdown/validate', { markdown }),

  apply: (markdown: string) =>
    api.post<ApplyResponse>('/markdown/apply', { markdown }),
};

// ── Audit Logs ────────────────────────────────────────────────────────────────

export interface LogListParams {
  limit?: number;
  offset?: number;
  from?: string;
  to?: string;
  operator?: string;
  action?: string;
  status?: string;
}

export const logsApi = {
  list: (params: LogListParams) =>
    api.get<PaginatedResponse<AuditLog>>('/logs', { params }),

  getById: (id: number) => api.get<AuditLog>(`/logs/${id}`),
};

// ── Integrity ─────────────────────────────────────────────────────────────────

export const integrityApi = {
  report: () => api.get<IntegrityReport>('/integrity/report'),

  reset: () =>
    api.post<{ total_employees: number; mismatches_found: number; baseline_updated: boolean }>(
      '/integrity/reset',
    ),
};

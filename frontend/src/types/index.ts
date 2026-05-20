export interface Employee {
  dn: string;
  fullName: string;
  email: string;
  office: string;
  telephoneNumber: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditLog {
  id: number;
  timestamp: string;
  operator: string;
  action: string;
  targetDN: string;
  attribute: string;
  oldValue: string;
  newValue: string;
  status: string;
  details: string;
  ipAddress: string;
}

export interface Mismatch {
  dn: string;
  old_hash: string;
  new_hash: string;
  checked_at: string;
}

export interface IntegrityReport {
  mismatches: Mismatch[];
  count: number;
}

export interface MarkdownOperation {
  dn: string;
  attribute: string;
  newValue: string;
  oldValue: string;
  valid: boolean;
  error?: string;
}

export interface ValidateResponse {
  valid: boolean;
  operations: MarkdownOperation[];
  errors: string[];
}

export interface OperationResult {
  dn: string;
  attribute: string;
  success: boolean;
  error?: string;
}

export interface ApplyResponse {
  applied: number;
  failed: number;
  details: OperationResult[];
}

export interface UserProfile {
  username: string;
  dn: string;
  groups: string[];
}

export interface UserPerms {
  isAdmin: boolean;
  isEditor: boolean;
}

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface Toast {
  id: string;
  type: ToastType;
  message: string;
}

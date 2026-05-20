import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { logsApi } from '../services/api';
import { LoadingSpinner } from '../components/LoadingSpinner';
import type { AuditLog } from '../types';
import type { LogListParams } from '../services/api';

const PAGE_SIZE = 50;
const ACTION_OPTIONS = ['', 'login', 'apply_markdown', 'update_employee', 'integrity_check'];
const STATUS_OPTIONS = ['', 'success', 'failure'];

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
        status === 'success'
          ? 'bg-green-900/50 text-green-300'
          : 'bg-red-900/50 text-red-300'
      }`}
    >
      {status}
    </span>
  );
}

function exportCsv(logs: AuditLog[]) {
  const headers = ['ID', 'Timestamp', 'Operator', 'Action', 'Target DN', 'Attribute', 'Old Value', 'New Value', 'Status', 'IP Address'];
  const rows = logs.map((l) => [
    l.id,
    l.timestamp,
    l.operator,
    l.action,
    l.targetDN,
    l.attribute,
    l.oldValue,
    l.newValue,
    l.status,
    l.ipAddress,
  ].map((v) => `"${String(v ?? '').replace(/"/g, '""')}"`).join(','));

  const csv = [headers.join(','), ...rows].join('\n');
  const blob = new Blob([csv], { type: 'text/csv' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

export function LogsPage() {
  const [offset, setOffset] = useState(0);
  const [filters, setFilters] = useState<Omit<LogListParams, 'limit' | 'offset'>>({});

  const { data, isLoading, isError } = useQuery({
    queryKey: ['logs', filters, offset],
    queryFn: () => logsApi.list({ ...filters, limit: PAGE_SIZE, offset }),
    placeholderData: (prev) => prev,
  });

  function setFilter<K extends keyof typeof filters>(k: K, v: (typeof filters)[K]) {
    setFilters((f) => ({ ...f, [k]: v || undefined }));
    setOffset(0);
  }

  const logs = data?.data.data ?? [];
  const total = data?.data.total ?? 0;
  const page = Math.floor(offset / PAGE_SIZE) + 1;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Audit Logs</h1>
          {!isLoading && <p className="mt-0.5 text-sm text-slate-400">{total} entries</p>}
        </div>
        <button
          onClick={() => exportCsv(logs)}
          disabled={logs.length === 0}
          className="rounded-md border border-slate-600 px-3 py-1.5 text-xs text-slate-300
            hover:bg-slate-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Export CSV
        </button>
      </div>

      {/* Filters */}
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          type="text"
          placeholder="Operator…"
          value={filters.operator ?? ''}
          onChange={(e) => setFilter('operator', e.target.value)}
          className="rounded-md border border-slate-600 bg-slate-800 px-3 py-1.5 text-xs
            text-slate-100 placeholder-slate-500 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 w-36"
        />
        <select
          value={filters.action ?? ''}
          onChange={(e) => setFilter('action', e.target.value)}
          className="rounded-md border border-slate-600 bg-slate-800 px-3 py-1.5 text-xs
            text-slate-100 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          {ACTION_OPTIONS.map((a) => (
            <option key={a} value={a}>{a || 'All actions'}</option>
          ))}
        </select>
        <select
          value={filters.status ?? ''}
          onChange={(e) => setFilter('status', e.target.value)}
          className="rounded-md border border-slate-600 bg-slate-800 px-3 py-1.5 text-xs
            text-slate-100 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>{s || 'All statuses'}</option>
          ))}
        </select>
        <input
          type="datetime-local"
          value={filters.from?.slice(0, 16) ?? ''}
          onChange={(e) =>
            setFilter('from', e.target.value ? new Date(e.target.value).toISOString() : undefined)
          }
          className="rounded-md border border-slate-600 bg-slate-800 px-3 py-1.5 text-xs
            text-slate-100 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          title="From date"
        />
        <input
          type="datetime-local"
          value={filters.to?.slice(0, 16) ?? ''}
          onChange={(e) =>
            setFilter('to', e.target.value ? new Date(e.target.value).toISOString() : undefined)
          }
          className="rounded-md border border-slate-600 bg-slate-800 px-3 py-1.5 text-xs
            text-slate-100 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          title="To date"
        />
        {Object.values(filters).some(Boolean) && (
          <button
            onClick={() => { setFilters({}); setOffset(0); }}
            className="rounded-md border border-slate-600 px-3 py-1.5 text-xs text-slate-400
              hover:bg-slate-700 hover:text-slate-200 transition-colors"
          >
            Clear filters
          </button>
        )}
      </div>

      {isError && (
        <div className="mb-4 rounded-lg border border-red-500/30 bg-red-950/40 px-4 py-3 text-sm text-red-300">
          Failed to load logs.
        </div>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-700">
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-slate-700 bg-slate-800/60">
                {['Timestamp', 'Operator', 'Action', 'Target DN', 'Attribute', 'Old → New', 'Status', 'IP'].map((h) => (
                  <th key={h} className="whitespace-nowrap px-3 py-3 text-left font-medium text-slate-400 uppercase tracking-wide">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr>
                  <td colSpan={8} className="py-12 text-center">
                    <LoadingSpinner size="md" className="mx-auto text-blue-500" />
                  </td>
                </tr>
              ) : logs.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-12 text-center text-slate-500">
                    No audit logs found.
                  </td>
                </tr>
              ) : (
                logs.map((log) => (
                  <tr key={log.id} className="border-b border-slate-700/50 hover:bg-slate-800/30">
                    <td className="whitespace-nowrap px-3 py-2 font-mono text-slate-400">
                      {new Date(log.timestamp).toLocaleString()}
                    </td>
                    <td className="px-3 py-2 text-slate-300">{log.operator}</td>
                    <td className="px-3 py-2">
                      <span className="rounded bg-slate-700 px-1.5 py-0.5 text-slate-300">
                        {log.action}
                      </span>
                    </td>
                    <td className="max-w-xs truncate px-3 py-2 font-mono text-slate-400" title={log.targetDN}>
                      {log.targetDN || '—'}
                    </td>
                    <td className="px-3 py-2 text-slate-300">{log.attribute || '—'}</td>
                    <td className="px-3 py-2 text-slate-400">
                      {log.oldValue || log.newValue ? (
                        <span>
                          <span className="text-red-400/80">{log.oldValue || '∅'}</span>
                          {' → '}
                          <span className="text-green-400/80">{log.newValue || '∅'}</span>
                        </span>
                      ) : '—'}
                    </td>
                    <td className="px-3 py-2">
                      <StatusBadge status={log.status} />
                    </td>
                    <td className="whitespace-nowrap px-3 py-2 font-mono text-slate-500">{log.ipAddress || '—'}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="mt-4 flex items-center justify-between">
        <span className="text-xs text-slate-500">Page {page} of {totalPages}</span>
        <div className="flex gap-2">
          <button
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            className="rounded-md border border-slate-600 px-3 py-1.5 text-xs text-slate-300
              hover:bg-slate-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            ← Previous
          </button>
          <button
            disabled={offset + PAGE_SIZE >= total}
            onClick={() => setOffset(offset + PAGE_SIZE)}
            className="rounded-md border border-slate-600 px-3 py-1.5 text-xs text-slate-300
              hover:bg-slate-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            Next →
          </button>
        </div>
      </div>
    </div>
  );
}

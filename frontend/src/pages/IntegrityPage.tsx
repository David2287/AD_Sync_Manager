import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { integrityApi } from '../services/api';
import { useToast } from '../hooks/useToast';
import { LoadingSpinner } from '../components/LoadingSpinner';

export function IntegrityPage() {
  const { addToast } = useToast();
  const qc = useQueryClient();

  const { data, isLoading, isError, dataUpdatedAt } = useQuery({
    queryKey: ['integrity-report'],
    queryFn: () => integrityApi.report(),
    refetchInterval: 60_000,
  });

  const resetMutation = useMutation({
    mutationFn: () => integrityApi.reset(),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['integrity-report'] });
      const { total_employees, mismatches_found } = res.data;
      addToast(
        'success',
        `Baseline reset. ${total_employees} employees checked, ${mismatches_found} mismatches found before reset.`,
      );
    },
    onError: () => addToast('error', 'Reset failed. A check may already be in progress.'),
  });

  const report = data?.data;
  const mismatches = report?.mismatches ?? [];

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Integrity Check</h1>
          {dataUpdatedAt > 0 && (
            <p className="mt-0.5 text-sm text-slate-400">
              Last fetched {new Date(dataUpdatedAt).toLocaleTimeString()}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => qc.invalidateQueries({ queryKey: ['integrity-report'] })}
            disabled={isLoading}
            className="flex items-center gap-1.5 rounded-md border border-slate-600 px-3 py-1.5
              text-xs text-slate-300 hover:bg-slate-700 disabled:opacity-50 transition-colors"
          >
            {isLoading && <LoadingSpinner size="sm" />}
            Refresh report
          </button>
          <button
            onClick={() => resetMutation.mutate()}
            disabled={resetMutation.isPending}
            className="flex items-center gap-1.5 rounded-md border border-amber-600/50 bg-amber-900/20
              px-3 py-1.5 text-xs font-medium text-amber-300 hover:bg-amber-900/40
              disabled:opacity-50 transition-colors"
          >
            {resetMutation.isPending && <LoadingSpinner size="sm" />}
            Reset baseline
          </button>
        </div>
      </div>

      {isError && (
        <div className="mb-4 rounded-lg border border-red-500/30 bg-red-950/40 px-4 py-3 text-sm text-red-300">
          Failed to load integrity report.
        </div>
      )}

      {/* Summary cards */}
      <div className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-3">
        <div className="rounded-lg border border-slate-700 bg-slate-800 p-4">
          <p className="text-xs text-slate-400 uppercase tracking-wide">Mismatches Found</p>
          <p className={`mt-1 text-3xl font-bold ${
            mismatches.length === 0 ? 'text-green-400' : 'text-red-400'
          }`}>
            {isLoading ? '—' : mismatches.length}
          </p>
        </div>
        <div className="rounded-lg border border-slate-700 bg-slate-800 p-4">
          <p className="text-xs text-slate-400 uppercase tracking-wide">Status</p>
          <p className={`mt-1 text-lg font-semibold ${
            mismatches.length === 0 ? 'text-green-400' : 'text-red-400'
          }`}>
            {isLoading ? '—' : mismatches.length === 0 ? 'Clean' : 'Violations'}
          </p>
        </div>
        <div className="rounded-lg border border-slate-700 bg-slate-800 p-4">
          <p className="text-xs text-slate-400 uppercase tracking-wide">Auto-update</p>
          <p className="mt-1 text-lg font-semibold text-slate-300">
            See server config
          </p>
        </div>
      </div>

      {/* Mismatches table */}
      {!isLoading && mismatches.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-slate-700 bg-slate-800/40 py-16">
          <div className="mb-2 text-3xl">✅</div>
          <p className="text-sm font-medium text-slate-300">No integrity violations detected</p>
          <p className="mt-1 text-xs text-slate-500">All employee records match their stored baseline.</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-slate-700">
          <div className="border-b border-slate-700 bg-slate-800/60 px-4 py-2 text-xs font-medium text-slate-400">
            Mismatches — {mismatches.length} record(s) changed outside the application
          </div>
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-slate-700">
                {['Distinguished Name', 'Old Hash', 'New Hash', 'Detected At'].map((h) => (
                  <th key={h} className="px-4 py-2 text-left font-medium text-slate-400">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {mismatches.map((m, i) => (
                <tr key={i} className="border-b border-slate-700/50 hover:bg-slate-800/30">
                  <td className="max-w-xs truncate px-4 py-2 font-mono text-red-300" title={m.dn}>
                    {m.dn}
                  </td>
                  <td className="px-4 py-2 font-mono text-slate-500">{m.old_hash.slice(0, 16)}…</td>
                  <td className="px-4 py-2 font-mono text-amber-300">{m.new_hash.slice(0, 16)}…</td>
                  <td className="whitespace-nowrap px-4 py-2 text-slate-400">
                    {new Date(m.checked_at).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="mt-6 rounded-lg border border-slate-700/50 bg-slate-800/30 p-4 text-xs text-slate-500">
        <strong className="text-slate-400">About:</strong> The integrity checker runs periodically (configured by{' '}
        <code>INTEGRITY_INTERVAL</code>) and compares a SHA-256 hash of each employee's key attributes against the stored
        baseline. Mismatches indicate changes made directly in AD (e.g., via ADUC or scripts) that bypassed this
        application. Use <em>Reset baseline</em> after acknowledging legitimate bulk changes.
      </div>
    </div>
  );
}

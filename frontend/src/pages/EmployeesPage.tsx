import { useState, useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { employeeApi } from '../services/api';
import { useAuth } from '../context/AuthContext';
import { useDebounce } from '../hooks/useDebounce';
import { useToast } from '../hooks/useToast';
import { LoadingSpinner } from '../components/LoadingSpinner';
import type { Employee } from '../types';

const PAGE_SIZE = 25;

interface EditState {
  dn: string;
  field: 'office' | 'telephoneNumber';
  value: string;
}

function InlineCell({
  value,
  canEdit,
  onSave,
}: {
  value: string;
  canEdit: boolean;
  onSave: (v: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) inputRef.current?.focus();
  }, [editing]);

  function commit() {
    setEditing(false);
    if (draft !== value) onSave(draft);
  }

  if (!editing) {
    return (
      <span
        onClick={() => canEdit && setEditing(true)}
        className={`block rounded px-1 py-0.5 ${
          canEdit
            ? 'cursor-pointer hover:bg-slate-700/60 hover:ring-1 hover:ring-slate-600'
            : ''
        }`}
        title={canEdit ? 'Click to edit' : undefined}
      >
        {value || <span className="text-slate-500">—</span>}
      </span>
    );
  }

  return (
    <input
      ref={inputRef}
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') commit();
        if (e.key === 'Escape') { setDraft(value); setEditing(false); }
      }}
      className="w-full rounded border border-blue-500 bg-slate-700 px-1 py-0.5 text-sm
        text-slate-100 outline-none ring-1 ring-blue-500"
    />
  );
}

export function EmployeesPage() {
  const { perms } = useAuth();
  const { addToast } = useToast();
  const qc = useQueryClient();

  const [search, setSearch] = useState('');
  const [offset, setOffset] = useState(0);
  const debouncedSearch = useDebounce(search, 350);
  const [savingDN, setSavingDN] = useState<string | null>(null);

  useEffect(() => setOffset(0), [debouncedSearch]);

  const { data, isLoading, isError } = useQuery({
    queryKey: ['employees', debouncedSearch, offset],
    queryFn: () =>
      employeeApi.list({ limit: PAGE_SIZE, offset, search: debouncedSearch || undefined }),
    placeholderData: (prev) => prev,
  });

  const updateMutation = useMutation({
    mutationFn: ({ dn, field, value }: EditState) =>
      employeeApi.update(dn, field === 'office' ? { office: value } : { telephoneNumber: value }),
    onSuccess: (_, vars) => {
      addToast('success', 'Employee updated successfully.');
      qc.invalidateQueries({ queryKey: ['employees'] });
      setSavingDN(null);
      void vars;
    },
    onError: () => {
      addToast('error', 'Failed to update employee. Check permissions.');
      setSavingDN(null);
    },
  });

  function handleSave(dn: string, field: 'office' | 'telephoneNumber', value: string) {
    setSavingDN(dn);
    updateMutation.mutate({ dn, field, value });
  }

  const employees = data?.data.data ?? [];
  const total = data?.data.total ?? 0;
  const page = Math.floor(offset / PAGE_SIZE) + 1;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Employees</h1>
          {!isLoading && (
            <p className="mt-0.5 text-sm text-slate-400">{total} total records</p>
          )}
        </div>
        <input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name or email…"
          className="w-64 rounded-md border border-slate-600 bg-slate-800 px-3 py-1.5 text-sm
            text-slate-100 placeholder-slate-500 outline-none focus:border-blue-500 focus:ring-1
            focus:ring-blue-500"
        />
      </div>

      {isError && (
        <div className="rounded-lg border border-red-500/30 bg-red-950/40 px-4 py-3 text-sm text-red-300">
          Failed to load employees. Check your connection.
        </div>
      )}

      <div className="overflow-hidden rounded-lg border border-slate-700">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-700 bg-slate-800/60">
              {['Full Name', 'Email', 'Office', 'Telephone', ''].map((h) => (
                <th key={h} className="px-4 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wide">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td colSpan={5} className="py-12 text-center">
                  <LoadingSpinner size="md" className="mx-auto text-blue-500" />
                </td>
              </tr>
            ) : employees.length === 0 ? (
              <tr>
                <td colSpan={5} className="py-12 text-center text-slate-500">
                  {debouncedSearch ? 'No employees match your search.' : 'No employees found.'}
                </td>
              </tr>
            ) : (
              employees.map((emp: Employee) => (
                <tr
                  key={emp.dn}
                  className="border-b border-slate-700/50 hover:bg-slate-800/40 transition-colors"
                >
                  <td className="px-4 py-3 font-medium text-slate-100">{emp.fullName}</td>
                  <td className="px-4 py-3 text-slate-300">{emp.email}</td>
                  <td className="px-4 py-3 text-slate-300 min-w-32">
                    <InlineCell
                      value={emp.office}
                      canEdit={perms.isEditor}
                      onSave={(v) => handleSave(emp.dn, 'office', v)}
                    />
                  </td>
                  <td className="px-4 py-3 text-slate-300 min-w-36">
                    <InlineCell
                      value={emp.telephoneNumber}
                      canEdit={perms.isEditor}
                      onSave={(v) => handleSave(emp.dn, 'telephoneNumber', v)}
                    />
                  </td>
                  <td className="px-4 py-3 text-right w-8">
                    {savingDN === emp.dn && <LoadingSpinner size="sm" className="text-blue-400" />}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <div className="mt-4 flex items-center justify-between">
        <span className="text-xs text-slate-500">
          Page {page} of {totalPages}
        </span>
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

      {perms.isEditor && (
        <p className="mt-3 text-xs text-slate-500">
          Click on an Office or Telephone cell to edit. Press Enter or click away to save.
        </p>
      )}
    </div>
  );
}

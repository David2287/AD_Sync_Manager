import { useState, useRef } from 'react';
import { useMutation } from '@tanstack/react-query';
import { markdownApi } from '../services/api';
import { useAuth } from '../context/AuthContext';
import { useToast } from '../hooks/useToast';
import { LoadingSpinner } from '../components/LoadingSpinner';
import type { MarkdownOperation, ApplyResponse } from '../types';

const EXAMPLE = `# Employee Data Corrections

## cn=jdoe,ou=Employees,dc=company,dc=com
- telephoneNumber: +1 555 123 4567
- physicalDeliveryOfficeName: B-202

## cn=jsmith,ou=Employees,dc=company,dc=com
- telephoneNumber: +1 555 987 6543
`;

function OperationsTable({ ops }: { ops: MarkdownOperation[] }) {
  return (
    <div className="overflow-hidden rounded-lg border border-slate-700">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-slate-700 bg-slate-800/60">
            {['DN', 'Attribute', 'Old Value', 'New Value', 'Valid'].map((h) => (
              <th key={h} className="px-3 py-2 text-left font-medium text-slate-400 uppercase tracking-wide">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {ops.map((op, i) => (
            <tr key={i} className="border-b border-slate-700/50 hover:bg-slate-800/30">
              <td className="px-3 py-2 font-mono text-slate-300 max-w-xs truncate" title={op.dn}>{op.dn}</td>
              <td className="px-3 py-2 text-slate-300">{op.attribute}</td>
              <td className="px-3 py-2 text-slate-400">{op.oldValue || <span className="text-slate-600">—</span>}</td>
              <td className="px-3 py-2 text-slate-100">{op.newValue}</td>
              <td className="px-3 py-2">
                {op.valid ? (
                  <span className="text-green-400 font-semibold">✓</span>
                ) : (
                  <span className="text-red-400" title={op.error}>✕</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ApplyResult({ result }: { result: ApplyResponse }) {
  return (
    <div>
      <div className="mb-3 flex gap-4">
        <div className="rounded-md border border-green-500/30 bg-green-950/40 px-3 py-2 text-sm">
          <span className="font-semibold text-green-400">{result.applied}</span>
          <span className="ml-1 text-green-600">applied</span>
        </div>
        <div className="rounded-md border border-red-500/30 bg-red-950/40 px-3 py-2 text-sm">
          <span className="font-semibold text-red-400">{result.failed}</span>
          <span className="ml-1 text-red-600">failed</span>
        </div>
      </div>
      {result.details.length > 0 && (
        <div className="overflow-hidden rounded-lg border border-slate-700">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-slate-700 bg-slate-800/60">
                {['DN', 'Attribute', 'Result'].map((h) => (
                  <th key={h} className="px-3 py-2 text-left font-medium text-slate-400">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.details.map((d, i) => (
                <tr key={i} className="border-b border-slate-700/50">
                  <td className="px-3 py-2 font-mono text-slate-300 max-w-xs truncate">{d.dn}</td>
                  <td className="px-3 py-2 text-slate-300">{d.attribute}</td>
                  <td className="px-3 py-2">
                    {d.success ? (
                      <span className="text-green-400">✓ Applied</span>
                    ) : (
                      <span className="text-red-400" title={d.error}>✕ {d.error}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export function MarkdownPage() {
  const { perms } = useAuth();
  const { addToast } = useToast();
  const [content, setContent] = useState(EXAMPLE);
  const [validateResult, setValidateResult] = useState<MarkdownOperation[] | null>(null);
  const [applyResult, setApplyResult] = useState<ApplyResponse | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const validateMutation = useMutation({
    mutationFn: () => markdownApi.validate(content),
    onSuccess: (res) => {
      setValidateResult(res.data.operations);
      setApplyResult(null);
      if (res.data.valid) {
        addToast('success', `Valid — ${res.data.operations.length} operation(s) parsed.`);
      } else {
        addToast('warning', `${res.data.errors.length} validation error(s) found.`);
      }
    },
    onError: () => addToast('error', 'Validation failed. Check the document format.'),
  });

  const applyMutation = useMutation({
    mutationFn: () => markdownApi.apply(content),
    onSuccess: (res) => {
      setApplyResult(res.data);
      setValidateResult(null);
      setShowConfirm(false);
      const { applied, failed } = res.data;
      addToast(
        failed === 0 ? 'success' : 'warning',
        `Applied ${applied} / ${applied + failed} operation(s).`,
      );
    },
    onError: () => { setShowConfirm(false); addToast('error', 'Apply failed.'); },
  });

  function loadFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => setContent(ev.target?.result as string ?? '');
    reader.readAsText(file);
    e.target.value = '';
  }

  function saveFile() {
    const blob = new Blob([content], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'corrections.md';
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="flex h-full flex-col">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-slate-100">Markdown Corrections</h1>
        <div className="flex gap-2">
          <input ref={fileRef} type="file" accept=".md,.txt" className="hidden" onChange={loadFile} />
          <button
            onClick={() => fileRef.current?.click()}
            className="rounded-md border border-slate-600 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 transition-colors"
          >
            Load file
          </button>
          <button
            onClick={saveFile}
            className="rounded-md border border-slate-600 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 transition-colors"
          >
            Save file
          </button>
          <button
            onClick={() => validateMutation.mutate()}
            disabled={validateMutation.isPending || !content.trim()}
            className="flex items-center gap-1.5 rounded-md border border-blue-600 px-3 py-1.5 text-xs
              font-medium text-blue-300 hover:bg-blue-600/20 disabled:opacity-50 transition-colors"
          >
            {validateMutation.isPending && <LoadingSpinner size="sm" />}
            Validate
          </button>
          {perms.isEditor && (
            <button
              onClick={() => setShowConfirm(true)}
              disabled={applyMutation.isPending || !content.trim()}
              className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs
                font-medium text-white hover:bg-blue-500 disabled:opacity-50 transition-colors"
            >
              {applyMutation.isPending && <LoadingSpinner size="sm" />}
              Apply
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-1 gap-4 overflow-hidden min-h-0">
        {/* Editor */}
        <div className="flex flex-1 flex-col overflow-hidden rounded-lg border border-slate-700">
          <div className="border-b border-slate-700 bg-slate-800/60 px-3 py-1.5 text-xs font-medium text-slate-400">
            Editor
          </div>
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            spellCheck={false}
            className="flex-1 resize-none bg-slate-900 px-4 py-3 font-mono text-sm text-slate-100
              outline-none placeholder-slate-600 leading-relaxed"
            placeholder="Paste your Markdown correction document here…"
          />
        </div>

        {/* Preview */}
        <div className="flex flex-1 flex-col overflow-hidden rounded-lg border border-slate-700">
          <div className="border-b border-slate-700 bg-slate-800/60 px-3 py-1.5 text-xs font-medium text-slate-400">
            {applyResult ? 'Apply Result' : 'Validation Preview'}
          </div>
          <div className="flex-1 overflow-y-auto p-4">
            {validateMutation.isPending || applyMutation.isPending ? (
              <div className="flex h-full items-center justify-center">
                <LoadingSpinner size="md" className="text-blue-500" />
              </div>
            ) : applyResult ? (
              <ApplyResult result={applyResult} />
            ) : validateResult !== null ? (
              validateResult.length === 0 ? (
                <p className="text-sm text-slate-500">No operations parsed.</p>
              ) : (
                <OperationsTable ops={validateResult} />
              )
            ) : (
              <p className="text-sm text-slate-500">
                Click <strong className="text-slate-400">Validate</strong> to preview parsed operations.
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Confirm modal */}
      {showConfirm && (
        <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="w-full max-w-sm rounded-xl border border-slate-700 bg-slate-800 p-6 shadow-2xl">
            <h2 className="mb-2 text-base font-semibold text-slate-100">Apply corrections?</h2>
            <p className="mb-6 text-sm text-slate-400">
              This will write changes to Active Directory. Make sure you have validated the document first.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowConfirm(false)}
                className="rounded-md border border-slate-600 px-4 py-2 text-sm text-slate-300 hover:bg-slate-700 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => applyMutation.mutate()}
                disabled={applyMutation.isPending}
                className="flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm
                  font-medium text-white hover:bg-blue-500 disabled:opacity-60 transition-colors"
              >
                {applyMutation.isPending && <LoadingSpinner size="sm" />}
                Confirm Apply
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

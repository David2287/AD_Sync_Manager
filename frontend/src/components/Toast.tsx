import React, { useState, useCallback } from 'react';
import { ToastContext } from '../hooks/useToast';
import type { Toast, ToastType } from '../types';

function ToastIcon({ type }: { type: ToastType }) {
  switch (type) {
    case 'success': return <span className="text-green-400">✓</span>;
    case 'error':   return <span className="text-red-400">✕</span>;
    case 'warning': return <span className="text-amber-400">⚠</span>;
    case 'info':    return <span className="text-blue-400">ℹ</span>;
  }
}

const toastBg: Record<ToastType, string> = {
  success: 'border-green-500/30 bg-green-950/80',
  error:   'border-red-500/30 bg-red-950/80',
  warning: 'border-amber-500/30 bg-amber-950/80',
  info:    'border-blue-500/30 bg-blue-950/80',
};

function ToastItem({ toast, onRemove }: { toast: Toast; onRemove: () => void }) {
  return (
    <div
      className={`flex items-start gap-3 rounded-lg border px-4 py-3 shadow-xl backdrop-blur-sm
        text-sm text-slate-100 min-w-64 max-w-sm ${toastBg[toast.type]}`}
    >
      <ToastIcon type={toast.type} />
      <span className="flex-1 leading-snug">{toast.message}</span>
      <button
        onClick={onRemove}
        className="ml-2 text-slate-400 hover:text-slate-200 transition-colors leading-none"
        aria-label="Dismiss"
      >
        ×
      </button>
    </div>
  );
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((type: ToastType, message: string) => {
    const id = `${Date.now()}-${Math.random()}`;
    setToasts((prev) => [...prev, { id, type, message }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 5000);
  }, []);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ toasts, addToast, removeToast }}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((t) => (
          <ToastItem key={t.id} toast={t} onRemove={() => removeToast(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

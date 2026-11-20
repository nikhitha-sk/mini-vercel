"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { CheckCircle2, Info, X, XCircle } from "lucide-react";
import { Button } from "./ui";
import { cn } from "@/lib/format";

/* ---------- Toasts ---------- */

type Tone = "success" | "error" | "info";
interface Toast {
  id: number;
  tone: Tone;
  title: string;
  description?: string;
}

interface ConfirmOptions {
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
}

interface FeedbackCtx {
  toast: (tone: Tone, title: string, description?: string) => void;
  confirm: (opts: ConfirmOptions) => Promise<boolean>;
}

const Ctx = createContext<FeedbackCtx | null>(null);

export function useFeedback() {
  const c = useContext(Ctx);
  if (!c) throw new Error("FeedbackProvider missing");
  return c;
}

export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [dialog, setDialog] = useState<(ConfirmOptions & { resolve: (v: boolean) => void }) | null>(null);
  const nextId = useRef(1);

  const toast = useCallback((tone: Tone, title: string, description?: string) => {
    const id = nextId.current++;
    setToasts((t) => [...t, { id, tone, title, description }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4500);
  }, []);

  const confirm = useCallback(
    (opts: ConfirmOptions) => new Promise<boolean>((resolve) => setDialog({ ...opts, resolve })),
    [],
  );

  const close = (v: boolean) => {
    dialog?.resolve(v);
    setDialog(null);
  };

  return (
    <Ctx.Provider value={{ toast, confirm }}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-[calc(100%-2rem)] max-w-sm flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className="animate-fade-in pointer-events-auto flex items-start gap-3 rounded-lg border border-line bg-elevated p-4 shadow-pop"
          >
            {t.tone === "success" && <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-ok" />}
            {t.tone === "error" && <XCircle className="mt-0.5 size-4 shrink-0 text-err" />}
            {t.tone === "info" && <Info className="mt-0.5 size-4 shrink-0 text-info" />}
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-medium">{t.title}</p>
              {t.description && <p className="mt-0.5 break-words text-[13px] text-muted">{t.description}</p>}
            </div>
            <button
              onClick={() => setToasts((x) => x.filter((y) => y.id !== t.id))}
              className="cursor-pointer text-faint hover:text-fg"
              aria-label="Dismiss"
            >
              <X className="size-4" />
            </button>
          </div>
        ))}
      </div>
      {dialog && <ConfirmDialog {...dialog} onClose={close} />}
    </Ctx.Provider>
  );
}

function ConfirmDialog({ title, description, confirmLabel = "Confirm", danger, onClose }: ConfirmOptions & { onClose: (v: boolean) => void }) {
  const ref = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    ref.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-[2px]" onClick={() => onClose(false)}>
      <div
        role="dialog"
        aria-modal="true"
        className="animate-fade-in w-full max-w-md overflow-hidden rounded-xl border border-line bg-elevated shadow-pop"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="p-6">
          <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
          {description && <div className="mt-2 text-sm leading-relaxed text-muted">{description}</div>}
        </div>
        <div className="flex justify-end gap-2 border-t border-line bg-subtle px-6 py-4">
          <Button variant="secondary" onClick={() => onClose(false)}>
            Cancel
          </Button>
          <Button ref={ref} variant={danger ? "danger" : "primary"} onClick={() => onClose(true)}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}

/* ---------- Copy button ---------- */

export function CopyButton({ value, className }: { value: string; className?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className={cn("cursor-pointer rounded px-1.5 py-0.5 text-xs text-faint transition-colors hover:bg-hover hover:text-fg", className)}
      onClick={() => {
        navigator.clipboard?.writeText(value);
        setDone(true);
        setTimeout(() => setDone(false), 1400);
      }}
    >
      {done ? "Copied" : "Copy"}
    </button>
  );
}

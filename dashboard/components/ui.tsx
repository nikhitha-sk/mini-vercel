"use client";

import Link from "next/link";
import { forwardRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";
import { Loader2 } from "lucide-react";
import type { Status } from "@/lib/api";
import { cn } from "@/lib/format";

/* ---------- Logo ---------- */

export function Logo({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="8" fill="currentColor" />
      <path d="M9 21.5 16 9l7 12.5" fill="none" stroke="var(--bg)" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M12.2 21.5h7.6" stroke="var(--bg)" strokeWidth="2.6" strokeLinecap="round" />
    </svg>
  );
}

/* ---------- Button ---------- */

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md";

const variants: Record<Variant, string> = {
  primary: "bg-accent text-accent-fg hover:opacity-90 border border-transparent",
  secondary: "bg-elevated text-fg border border-line hover:bg-hover hover:border-line-strong",
  ghost: "text-muted hover:text-fg hover:bg-hover border border-transparent",
  danger: "bg-err text-white hover:opacity-90 border border-transparent",
};
const sizes: Record<Size, string> = {
  sm: "h-8 px-3 text-[13px] gap-1.5",
  md: "h-10 px-4 text-sm gap-2",
};

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  icon?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", loading, icon, className, children, disabled, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={cn(
        "inline-flex items-center justify-center rounded-md font-medium transition-all duration-150 whitespace-nowrap",
        "disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer select-none",
        variants[variant],
        sizes[size],
        className,
      )}
      {...rest}
    >
      {loading ? <Loader2 className="size-4 animate-spin" /> : icon}
      {children}
    </button>
  );
});

export function ButtonLink({
  href,
  variant = "secondary",
  size = "md",
  icon,
  className,
  children,
  external,
}: {
  href: string;
  variant?: Variant;
  size?: Size;
  icon?: ReactNode;
  className?: string;
  children?: ReactNode;
  external?: boolean;
}) {
  const cls = cn(
    "inline-flex items-center justify-center rounded-md font-medium transition-all duration-150 whitespace-nowrap",
    variants[variant],
    sizes[size],
    className,
  );
  if (external)
    return (
      <a href={href} target="_blank" rel="noreferrer" className={cls}>
        {icon}
        {children}
      </a>
    );
  return (
    <Link href={href} className={cls}>
      {icon}
      {children}
    </Link>
  );
}

/* ---------- Card ---------- */

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn("rounded-xl border border-line bg-elevated shadow-card", className)}>{children}</div>;
}

export function CardHeader({ title, description, actions }: { title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 border-b border-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <h3 className="text-[15px] font-semibold tracking-tight">{title}</h3>
        {description && <p className="mt-0.5 text-[13px] text-muted">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

/* ---------- Form controls ---------- */

export function Label({ children, htmlFor, hint }: { children: ReactNode; htmlFor?: string; hint?: ReactNode }) {
  return (
    <label htmlFor={htmlFor} className="mb-1.5 flex items-baseline justify-between gap-2 text-[13px] font-medium text-fg">
      <span>{children}</span>
      {hint && <span className="text-xs font-normal text-faint">{hint}</span>}
    </label>
  );
}

const fieldCls =
  "w-full rounded-md border border-line bg-elevated px-3 text-sm text-fg placeholder:text-faint transition-colors " +
  "hover:border-line-strong focus:border-ring focus:outline-none focus:ring-3 focus:ring-ring/20 disabled:opacity-60";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { mono?: boolean }>(function Input(
  { className, mono, ...rest },
  ref,
) {
  return <input ref={ref} className={cn(fieldCls, "h-10", mono && "font-mono text-[13px]", className)} {...rest} />;
});

export function Select({ className, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cn(fieldCls, "h-10 cursor-pointer appearance-none bg-[length:16px] bg-[right_10px_center] bg-no-repeat pr-9", className)}
      style={{ backgroundImage: "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23888' stroke-width='2'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")" }}
      {...rest}>
      {children}
    </select>
  );
}

/* ---------- Status ---------- */

const statusStyle: Record<Status, { dot: string; text: string; label: string; pulse?: boolean }> = {
  QUEUED: { dot: "bg-faint", text: "text-muted", label: "Queued", pulse: true },
  BUILDING: { dot: "bg-warn", text: "text-warn", label: "Building", pulse: true },
  UPLOADING: { dot: "bg-info", text: "text-info", label: "Uploading", pulse: true },
  READY: { dot: "bg-ok", text: "text-ok", label: "Ready" },
  FAILED: { dot: "bg-err", text: "text-err", label: "Error" },
  CANCELED: { dot: "bg-faint", text: "text-faint", label: "Canceled" },
};

export function StatusDot({ status, className }: { status: Status; className?: string }) {
  const s = statusStyle[status];
  return <span className={cn("inline-block size-2.5 shrink-0 rounded-full", s.dot, s.pulse && "animate-pulse-dot", className)} />;
}

export function StatusBadge({ status, size = "md" }: { status: Status; size?: "sm" | "md" }) {
  const s = statusStyle[status];
  return (
    <span className={cn("inline-flex items-center gap-2 font-medium", s.text, size === "sm" ? "text-xs" : "text-[13px]")}>
      <StatusDot status={status} />
      {s.label}
    </span>
  );
}

export function Pill({ children, tone = "neutral", className }: { children: ReactNode; tone?: "neutral" | "ok" | "info" | "warn" | "err"; className?: string }) {
  const tones = {
    neutral: "bg-subtle text-muted border-line",
    ok: "bg-ok-bg text-ok border-transparent",
    info: "bg-info-bg text-info border-transparent",
    warn: "bg-warn-bg text-warn border-transparent",
    err: "bg-err-bg text-err border-transparent",
  };
  return (
    <span className={cn("inline-flex h-[22px] items-center gap-1 rounded-full border px-2 text-[11px] font-medium", tones[tone], className)}>
      {children}
    </span>
  );
}

/* ---------- Misc ---------- */

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn("size-4 animate-spin text-muted", className)} />;
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn("skeleton", className)} />;
}

export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-16 text-center">
      {icon && <div className="mb-4 flex size-12 items-center justify-center rounded-full border border-line bg-subtle text-muted">{icon}</div>}
      <h3 className="text-[15px] font-semibold">{title}</h3>
      {description && <p className="mt-1 max-w-sm text-[13px] leading-relaxed text-muted">{description}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

export function ErrorBanner({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-err/30 bg-err-bg px-4 py-3 text-[13px] text-err">
      <span>{message}</span>
      {onRetry && (
        <button onClick={onRetry} className="cursor-pointer font-medium underline underline-offset-2">
          Retry
        </button>
      )}
    </div>
  );
}

export function Kv({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="mb-1 text-xs font-medium text-faint">{label}</div>
      <div className={cn("truncate text-sm text-fg", mono && "font-mono text-[13px]")}>{children}</div>
    </div>
  );
}

export function Avatar({ name, className }: { name: string; className?: string }) {
  // Deterministic gradient per project so cards are easy to scan.
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) % 360;
  return (
    <div
      className={cn("flex size-8 shrink-0 items-center justify-center rounded-full text-[13px] font-semibold text-white", className)}
      style={{ background: `linear-gradient(135deg, hsl(${h} 70% 55%), hsl(${(h + 40) % 360} 75% 45%))` }}
    >
      {name.slice(0, 1).toUpperCase()}
    </div>
  );
}

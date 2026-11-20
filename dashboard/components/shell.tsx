"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { BookOpen, Monitor, Moon, Plus, Sun } from "lucide-react";
import { Logo, ButtonLink } from "./ui";
import { API_URL } from "@/lib/api";
import { cn } from "@/lib/format";

type Theme = "light" | "dark" | "system";

function applyTheme(t: Theme) {
  const dark = t === "dark" || (t === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>("system");
  useEffect(() => {
    let t: Theme = "system";
    try {
      t = (localStorage.getItem("mv_theme") as Theme) || "system";
    } catch {}
    setTheme(t);
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => applyTheme((localStorage.getItem("mv_theme") as Theme) || "system");
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);
  const set = (t: Theme) => {
    setTheme(t);
    try {
      localStorage.setItem("mv_theme", t);
    } catch {}
    applyTheme(t);
  };
  const opts: { v: Theme; icon: ReactNode; label: string }[] = [
    { v: "system", icon: <Monitor className="size-3.5" />, label: "System theme" },
    { v: "light", icon: <Sun className="size-3.5" />, label: "Light theme" },
    { v: "dark", icon: <Moon className="size-3.5" />, label: "Dark theme" },
  ];
  return (
    <div className="flex items-center rounded-full border border-line p-0.5">
      {opts.map((o) => (
        <button
          key={o.v}
          aria-label={o.label}
          title={o.label}
          onClick={() => set(o.v)}
          className={cn(
            "flex size-7 cursor-pointer items-center justify-center rounded-full transition-colors",
            theme === o.v ? "bg-hover text-fg" : "text-faint hover:text-fg",
          )}
        >
          {o.icon}
        </button>
      ))}
    </div>
  );
}

export interface Crumb {
  label: ReactNode;
  href?: string;
}

export interface Tab {
  label: string;
  href: string;
  active: boolean;
}

export function AppShell({ crumbs = [], tabs, children }: { crumbs?: Crumb[]; tabs?: Tab[]; children: ReactNode }) {
  const pathname = usePathname();
  const defaultTabs: Tab[] = [
    { label: "Overview", href: "/", active: pathname === "/" },
    { label: "Deployments", href: "/deployments", active: pathname.startsWith("/deployments") },
    { label: "Platform", href: "/platform", active: pathname.startsWith("/platform") },
  ];
  const navTabs = tabs ?? defaultTabs;
  const [online, setOnline] = useState<boolean | null>(null);
  useEffect(() => {
    let alive = true;
    const check = () =>
      fetch(`${API_URL}/api/health`)
        .then((r) => alive && setOnline(r.ok))
        .catch(() => alive && setOnline(false));
    check();
    const t = setInterval(check, 15000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-40 border-b border-line bg-elevated/85 backdrop-blur-md">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:px-6">
          <Link href="/" className="flex items-center gap-2.5 text-fg" aria-label="Mini Vercel home">
            <Logo />
            <span className="hidden text-[15px] font-semibold tracking-tight sm:inline">Mini Vercel</span>
          </Link>
          <span className="hidden text-line-strong sm:inline">/</span>
          <div className="flex min-w-0 items-center gap-1.5 text-sm">
            <span className="hidden items-center gap-2 rounded-md px-1.5 py-1 font-medium sm:flex">
              <span className="flex size-5 items-center justify-center rounded-full bg-gradient-to-br from-fuchsia-500 to-orange-400 text-[10px] font-bold text-white">
                N
              </span>
              Personal
            </span>
            {crumbs.map((c, i) => (
              <span key={i} className="flex min-w-0 items-center gap-1.5">
                <span className="hidden text-line-strong sm:inline">/</span>
                {c.href ? (
                  <Link href={c.href} className="truncate rounded-md px-1.5 py-1 font-medium hover:bg-hover">
                    {c.label}
                  </Link>
                ) : (
                  <span className="truncate px-1.5 py-1 font-medium">{c.label}</span>
                )}
              </span>
            ))}
          </div>
          <div className="ml-auto flex items-center gap-2">
            <a
              href="https://github.com/nikhitha-sk/mini-vercel#readme"
              target="_blank"
              rel="noreferrer"
              className="hidden items-center gap-1.5 rounded-md px-2 py-1.5 text-[13px] text-muted hover:text-fg md:flex"
            >
              <BookOpen className="size-3.5" /> Docs
            </a>
            <ThemeToggle />
            <ButtonLink href="/new" variant="primary" size="sm" icon={<Plus className="size-4" />} className="hidden sm:inline-flex">
              Add New
            </ButtonLink>
          </div>
        </div>
        <nav className="mx-auto flex max-w-6xl gap-1 overflow-x-auto px-2 sm:px-4">
          {navTabs.map((t) => (
            <Link
              key={t.href}
              href={t.href}
              className={cn(
                "relative whitespace-nowrap px-3 py-3 text-[13px] transition-colors",
                t.active ? "text-fg" : "text-muted hover:text-fg",
              )}
            >
              {t.label}
              {t.active && <span className="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-fg" />}
            </Link>
          ))}
        </nav>
      </header>

      {online === false && (
        <div className="border-b border-err/30 bg-err-bg px-4 py-2 text-center text-[13px] text-err">
          The API server at <span className="font-mono">{API_URL || "this origin"}</span> is unreachable. Start it with{" "}
          <span className="font-mono">go run ./cmd/api</span>.
        </div>
      )}

      <main className="flex-1">{children}</main>

      <footer className="border-t border-line">
        <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-2 px-4 py-6 text-xs text-faint sm:flex-row sm:px-6">
          <div className="flex items-center gap-2">
            <Logo size={16} />
            <span>Mini Vercel · static hosting platform</span>
          </div>
          <div className="flex items-center gap-2">
            <span className={cn("size-2 rounded-full", online ? "bg-ok" : online === false ? "bg-err" : "bg-faint")} />
            {online ? "All systems normal" : online === false ? "API offline" : "Checking status…"}
          </div>
        </div>
      </footer>
    </div>
  );
}

export function PageHeader({ title, description, actions }: { title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="border-b border-line bg-elevated">
      <div className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-8 sm:flex-row sm:items-center sm:justify-between sm:px-6 sm:py-10">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-semibold tracking-tight sm:text-[32px]">{title}</h1>
          {description && <div className="mt-1.5 text-sm text-muted">{description}</div>}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </div>
  );
}

export function Container({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("mx-auto max-w-6xl px-4 py-8 sm:px-6", className)}>{children}</div>;
}

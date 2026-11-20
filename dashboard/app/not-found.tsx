import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="flex items-center gap-6">
        <span className="border-r border-line pr-6 text-2xl font-semibold">404</span>
        <div>
          <h1 className="text-[15px] font-medium">This page could not be found.</h1>
          <Link href="/" className="mt-1 inline-block text-[13px] text-muted underline underline-offset-2 hover:text-fg">
            Back to dashboard
          </Link>
        </div>
      </div>
    </div>
  );
}

"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

const TABS = [
  { href: "/", label: "本周菜单" },
  { href: "/shopping", label: "买菜清单" },
  { href: "/inbox", label: "建议箱" },
  { href: "/settings", label: "设置" },
];

export default function Nav() {
  const pathname = usePathname();
  const router = useRouter();
  if (pathname.startsWith("/login") || pathname.startsWith("/maid")) return null;

  const logout = () => {
    localStorage.removeItem("token");
    router.push("/login");
  };

  return (
    <nav className="sticky top-0 z-10 mb-6 border-b border-stone-200 bg-white/90 backdrop-blur">
      <div className="mx-auto flex max-w-3xl items-center gap-1 overflow-x-auto px-4 py-3">
        <span className="mr-3 shrink-0 text-lg font-bold text-emerald-700">膳计</span>
        {TABS.map((t) => (
          <Link
            key={t.href}
            href={t.href}
            className={`shrink-0 rounded-full px-3 py-1.5 text-sm ${
              pathname === t.href
                ? "bg-emerald-600 font-medium text-white"
                : "text-stone-600 hover:bg-stone-100"
            }`}
          >
            {t.label}
          </Link>
        ))}
        <button
          onClick={logout}
          className="ml-auto shrink-0 rounded-full px-3 py-1.5 text-sm text-stone-400 hover:bg-stone-100"
        >
          退出
        </button>
      </div>
    </nav>
  );
}

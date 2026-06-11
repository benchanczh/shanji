"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { api, ApiError, mondayOf, CATEGORY_LABELS, type PlanView, type ShoppingItem } from "@/lib/api";

export default function ShoppingPage() {
  return (
    <Suspense>
      <ShoppingInner />
    </Suspense>
  );
}

function ShoppingInner() {
  const params = useSearchParams();
  const week = params.get("week") ?? mondayOf(new Date());
  const [items, setItems] = useState<ShoppingItem[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    (async () => {
      try {
        const plan = await api<PlanView>("GET", `/plans?week_start=${week}`);
        const list = await api<{ id: number; items: ShoppingItem[] }>("GET", `/plans/${plan.id}/shopping-list`);
        setItems(list.items);
      } catch (err) {
        setError(
          err instanceof ApiError && err.status === 404
            ? "本周还没有买菜清单——先在「本周菜单」生成并确认菜单"
            : err instanceof Error ? err.message : "加载失败"
        );
      }
    })();
  }, [week]);

  const toggle = async (item: ShoppingItem) => {
    setItems((prev) => prev!.map((i) => (i.id === item.id ? { ...i, checked: !i.checked } : i)));
    try {
      await api("PATCH", `/shopping-items/${item.id}`, { checked: !item.checked });
    } catch {
      setItems((prev) => prev!.map((i) => (i.id === item.id ? { ...i, checked: item.checked } : i)));
    }
  };

  const groups = new Map<string, ShoppingItem[]>();
  for (const it of items ?? []) {
    const list = groups.get(it.category) ?? [];
    list.push(it);
    groups.set(it.category, list);
  }
  const done = (items ?? []).filter((i) => i.checked).length;

  return (
    <div>
      <div className="mb-4 flex items-baseline gap-3">
        <h1 className="text-xl font-bold">买菜清单</h1>
        <span className="text-sm text-stone-400">{week} 起一周</span>
        {items && <span className="ml-auto text-sm text-stone-500">{done}/{items.length} 已买</span>}
      </div>

      {error && <p className="rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800">{error}</p>}

      <div className="space-y-4">
        {[...groups.entries()].map(([category, list]) => (
          <div key={category} className="rounded-2xl border border-stone-200 bg-white p-4 shadow-sm">
            <h2 className="mb-2 text-sm font-bold text-emerald-700">{CATEGORY_LABELS[category] ?? category}</h2>
            <ul className="divide-y divide-stone-100">
              {list.map((it) => (
                <li key={it.id} className="flex items-center gap-3 py-2">
                  <input
                    type="checkbox"
                    checked={it.checked}
                    onChange={() => toggle(it)}
                    className="h-5 w-5 accent-emerald-600"
                  />
                  <div className={it.checked ? "line-through opacity-40" : ""}>
                    <span className="font-medium">{it.name}</span>
                    <span className="ml-2 text-xs text-stone-400">{it.name_en}</span>
                  </div>
                  <span className={`ml-auto text-sm tabular-nums ${it.checked ? "opacity-40" : "text-stone-600"}`}>
                    {it.total_qty != null ? `${it.total_qty} ${it.unit}` : it.unit}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </div>
  );
}

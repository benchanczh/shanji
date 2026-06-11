"use client";

import { useEffect, useState } from "react";
import { api, type Suggestion } from "@/lib/api";

export default function InboxPage() {
  const [items, setItems] = useState<Suggestion[]>([]);
  const [error, setError] = useState("");

  const load = () => api<Suggestion[]>("GET", "/suggestions").then(setItems).catch((e) => setError(e.message));
  useEffect(() => { load(); }, []);

  const review = async (id: number, action: "approve" | "reject") => {
    try {
      await api("POST", `/suggestions/${id}/${action}`);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "操作失败");
    }
  };

  const pending = items.filter((s) => s.status === "pending");
  const reviewed = items.filter((s) => s.status !== "pending");

  return (
    <div>
      <h1 className="mb-4 text-xl font-bold">建议箱</h1>
      {error && <p className="mb-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>}

      {pending.length === 0 && <p className="rounded-2xl border border-dashed border-stone-300 py-12 text-center text-stone-400">没有待处理的建议</p>}

      <div className="space-y-3">
        {pending.map((s) => (
          <div key={s.id} className="rounded-2xl border border-stone-200 bg-white p-4 shadow-sm">
            <div className="mb-1 flex items-center gap-2">
              <span className="font-medium">{s.title}</span>
              <span className="rounded bg-stone-100 px-1.5 text-xs text-stone-500">{s.from_role === "helper" ? "阿姨" : "家人"}</span>
              <span className="ml-auto text-xs text-stone-400">{s.created_at.slice(0, 10)}</span>
            </div>
            {s.content && <p className="mb-3 text-sm text-stone-600">{s.content}</p>}
            <div className="flex gap-2">
              <button onClick={() => review(s.id, "approve")} className="rounded-lg bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-700">通过</button>
              <button onClick={() => review(s.id, "reject")} className="rounded-lg border border-stone-300 px-4 py-1.5 text-sm hover:bg-stone-100">拒绝</button>
            </div>
          </div>
        ))}
      </div>

      {reviewed.length > 0 && (
        <div className="mt-8">
          <h2 className="mb-2 text-sm font-medium text-stone-400">已处理</h2>
          <ul className="space-y-1">
            {reviewed.map((s) => (
              <li key={s.id} className="flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-sm text-stone-500">
                {s.title}
                <span className={`ml-auto rounded px-1.5 text-xs ${s.status === "approved" ? "bg-emerald-50 text-emerald-600" : "bg-stone-100 text-stone-400"}`}>
                  {s.status === "approved" ? "已通过" : "已拒绝"}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

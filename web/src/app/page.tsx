"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  api,
  ApiError,
  addDays,
  mondayOf,
  COURSE_LABELS,
  MEAL_LABELS,
  type DishView,
  type PlanView,
} from "@/lib/api";

const WEEKDAYS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];

export default function PlanBoard() {
  const [weekStart, setWeekStart] = useState(() => mondayOf(new Date()));
  const [plan, setPlan] = useState<PlanView | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async (ws: string) => {
    setError("");
    try {
      setPlan(await api<PlanView>("GET", `/plans?week_start=${ws}`));
    } catch (err) {
      setPlan(null);
      if (!(err instanceof ApiError && err.status === 404)) {
        setError(err instanceof Error ? err.message : "加载失败");
      }
    }
  }, []);

  useEffect(() => {
    if (!localStorage.getItem("token")) {
      window.location.href = "/login";
      return;
    }
    load(weekStart);
  }, [weekStart, load]);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    } finally {
      setBusy(false);
    }
  };

  const generate = () =>
    act(async () => {
      const data = await api<{ plan: PlanView; warnings?: string[] }>("POST", "/plans/generate", {
        week_start: weekStart,
      });
      setPlan(data.plan);
      setWarnings(data.warnings ?? []);
    });

  const swap = (dish: DishView) =>
    act(async () => {
      await api("POST", `/dishes/${dish.id}/swap`);
      await load(weekStart);
    });

  const toggleLock = (slotId: number, locked: boolean) =>
    act(async () => {
      await api("PATCH", `/slots/${slotId}`, { locked: !locked });
      await load(weekStart);
    });

  const confirm = () =>
    act(async () => {
      await api("POST", `/plans/${plan!.id}/confirm`);
      await load(weekStart);
      setWarnings([]);
    });

  const draft = plan?.status === "draft";

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <button onClick={() => setWeekStart(addDays(weekStart, -7))} className="rounded-lg border border-stone-300 px-2.5 py-1.5 text-sm hover:bg-stone-100">←</button>
        <span className="font-medium">{weekStart} 起一周</span>
        <button onClick={() => setWeekStart(addDays(weekStart, 7))} className="rounded-lg border border-stone-300 px-2.5 py-1.5 text-sm hover:bg-stone-100">→</button>
        {plan && (
          <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${draft ? "bg-amber-100 text-amber-700" : "bg-emerald-100 text-emerald-700"}`}>
            {draft ? "草稿" : "已确认"}
          </span>
        )}
        <div className="ml-auto flex gap-2">
          {(!plan || draft) && (
            <button onClick={generate} disabled={busy} className="rounded-lg bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50">
              {plan ? "重新生成" : "一键生成"}
            </button>
          )}
          {draft && (
            <button onClick={confirm} disabled={busy} className="rounded-lg bg-amber-500 px-4 py-1.5 text-sm font-medium text-white hover:bg-amber-600 disabled:opacity-50">
              确认菜单
            </button>
          )}
          {plan && !draft && (
            <Link href={`/shopping?week=${weekStart}`} className="rounded-lg bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-700">
              查看买菜清单
            </Link>
          )}
        </div>
      </div>

      {error && <p className="mb-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>}
      {warnings.length > 0 && (
        <div className="mb-3 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800">
          {warnings.map((w) => <p key={w}>⚠ {w}</p>)}
        </div>
      )}

      {!plan && (
        <div className="rounded-2xl border border-dashed border-stone-300 py-20 text-center text-stone-400">
          本周还没有菜单，点「一键生成」开始
        </div>
      )}

      <div className="space-y-4">
        {plan?.days.map((day, i) => (
          <div key={day.date} className="rounded-2xl border border-stone-200 bg-white p-4 shadow-sm">
            <div className="mb-3 flex items-baseline gap-2">
              <span className="font-bold">{WEEKDAYS[i] ?? day.date}</span>
              <span className="text-xs text-stone-400">{day.date}</span>
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              {(["breakfast", "lunch", "dinner"] as const).map((meal) => {
                const mv = day.meals[meal];
                if (!mv) return <div key={meal} />;
                return (
                  <div key={meal} className={`rounded-xl p-3 ${mv.locked ? "bg-amber-50 ring-1 ring-amber-200" : "bg-stone-50"}`}>
                    <div className="mb-2 flex items-center justify-between">
                      <span className="text-xs font-medium text-stone-500">{MEAL_LABELS[meal]}</span>
                      {draft && (
                        <button
                          onClick={() => toggleLock(mv.slot_id, mv.locked)}
                          disabled={busy}
                          title={mv.locked ? "解锁（重排时可变）" : "锁定（重排时保留）"}
                          className={`rounded px-1.5 text-xs ${mv.locked ? "text-amber-600" : "text-stone-300 hover:text-stone-500"}`}
                        >
                          {mv.locked ? "已锁" : "锁定"}
                        </button>
                      )}
                    </div>
                    <ul className="space-y-1.5">
                      {mv.dishes.map((d) => (
                        <li key={d.id} className="group flex items-center gap-1.5 text-sm">
                          {d.target === "baby" ? (
                            <span className="rounded bg-pink-100 px-1 text-[10px] text-pink-600">宝宝</span>
                          ) : (
                            <span className="rounded bg-stone-200 px-1 text-[10px] text-stone-500">{COURSE_LABELS[d.course]}</span>
                          )}
                          <span className={d.course === "main" && d.target === "adult" ? "font-medium" : ""}>{d.name}</span>
                          {draft && d.target === "adult" && (
                            <button
                              onClick={() => swap(d)}
                              disabled={busy}
                              title="换一个"
                              className="ml-auto rounded px-1 text-xs text-stone-300 hover:text-emerald-600 group-hover:text-stone-400"
                            >
                              换
                            </button>
                          )}
                        </li>
                      ))}
                    </ul>
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

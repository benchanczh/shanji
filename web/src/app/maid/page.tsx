"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { api, ApiError, type MaidDay, type MaidDish } from "@/lib/api";

export default function MaidPage() {
  return (
    <Suspense>
      <MaidInner />
    </Suspense>
  );
}

const MEALS_EN: Record<string, string> = { breakfast: "Breakfast", lunch: "Lunch", dinner: "Dinner" };

function MaidInner() {
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const [day, setDay] = useState<MaidDay | null>(null);
  const [error, setError] = useState("");
  const [sent, setSent] = useState(false);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      setDay(await api<MaidDay>("GET", `/maid/today?token=${token}`, undefined, { noAuth: true }));
    } catch (err) {
      setDay(null);
      setError(
        err instanceof ApiError && err.status === 404
          ? "No menu confirmed for today yet. Please check with the family."
          : err instanceof ApiError && err.status === 401
          ? "This link is no longer valid. Please ask for a new one."
          : "Something went wrong."
      );
    }
  }, [token]);

  useEffect(() => { load(); }, [load]);

  const suggest = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api("POST", `/maid/suggestions?token=${token}`, { title, content }, { noAuth: true });
      setSent(true);
      setTitle("");
      setContent("");
    } catch {
      setError("Could not send the suggestion. Please try again.");
    }
  };

  return (
    <div className="mx-auto max-w-2xl px-1 py-6">
      <header className="mb-6">
        <h1 className="text-2xl font-bold text-emerald-700">Today&apos;s cooking</h1>
        {day && <p className="text-sm text-stone-500">{day.date}</p>}
      </header>

      {error && <p className="rounded-xl bg-amber-50 px-4 py-3 text-amber-800">{error}</p>}

      {day &&
        (["breakfast", "lunch", "dinner"] as const).map((meal) => {
          const dishes = day.meals[meal];
          if (!dishes?.length) return null;
          return (
            <section key={meal} className="mb-8">
              <h2 className="mb-3 text-lg font-bold">{MEALS_EN[meal]}</h2>
              <div className="space-y-4">
                {dishes.filter((d) => d.target === "adult").map((d) => (
                  <DishCard key={`${d.recipe_id}-adult`} dish={d} babyDish={dishes.find((b) => b.target === "baby" && b.recipe_id === d.recipe_id)} />
                ))}
                {dishes.filter((d) => d.target === "baby" && !dishes.some((a) => a.target === "adult" && a.recipe_id === d.recipe_id)).map((d) => (
                  <DishCard key={`${d.recipe_id}-baby`} dish={d} babyOnly />
                ))}
              </div>
            </section>
          );
        })}

      {day && (
        <section className="mt-10 rounded-2xl border border-stone-200 bg-white p-5 shadow-sm">
          <h2 className="mb-1 font-bold">Suggest a dish</h2>
          <p className="mb-3 text-sm text-stone-500">Know a dish the family might like? Tell them here.</p>
          {sent ? (
            <p className="rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700">Sent! The family will review it. Thank you!</p>
          ) : (
            <form onSubmit={suggest} className="space-y-2">
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="Dish name (e.g. Chicken adobo)"
                className="w-full rounded-lg border border-stone-300 px-3 py-2 text-sm"
              />
              <textarea
                value={content}
                onChange={(e) => setContent(e.target.value)}
                placeholder="How is it cooked? Why might they like it?"
                rows={2}
                className="w-full rounded-lg border border-stone-300 px-3 py-2 text-sm"
              />
              <button disabled={!title} className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50">
                Send suggestion
              </button>
            </form>
          )}
        </section>
      )}
    </div>
  );
}

function DishCard({ dish, babyDish, babyOnly }: { dish: MaidDish; babyDish?: MaidDish; babyOnly?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="rounded-2xl border border-stone-200 bg-white shadow-sm">
      <button onClick={() => setOpen(!open)} className="flex w-full items-center gap-2 p-4 text-left">
        <div>
          <p className="font-medium">
            {dish.name_en || dish.name}
            {babyOnly && <span className="ml-2 rounded bg-pink-100 px-1.5 text-xs text-pink-600">for baby</span>}
            {babyDish && <span className="ml-2 rounded bg-pink-100 px-1.5 text-xs text-pink-600">+ baby portion</span>}
          </p>
          <p className="text-xs text-stone-400">{dish.name} · {dish.minutes} min</p>
        </div>
        <span className="ml-auto text-stone-300">{open ? "▴" : "▾"}</span>
      </button>
      {open && (
        <div className="border-t border-stone-100 p-4 pt-3">
          <h3 className="mb-1 text-xs font-bold uppercase tracking-wide text-stone-400">Ingredients</h3>
          <ul className="mb-3 grid grid-cols-2 gap-x-4 text-sm">
            {dish.ingredients.map((i, idx) => (
              <li key={idx} className="flex justify-between border-b border-dotted border-stone-200 py-0.5">
                <span>{i.name_en || i.name}</span>
                <span className="text-stone-500">{i.qty != null ? `${i.qty} ${i.unit}` : "to taste"}</span>
              </li>
            ))}
          </ul>
          <h3 className="mb-1 text-xs font-bold uppercase tracking-wide text-stone-400">Steps</h3>
          <ol className="space-y-2 text-sm">
            {dish.steps.map((s) => (
              <li key={s.order} className="flex gap-2">
                <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-xs font-bold text-emerald-700">{s.order}</span>
                <div>
                  {s.baby_split_point && (
                    <p className="mb-0.5 rounded bg-pink-50 px-1.5 py-0.5 text-xs text-pink-600">
                      Before this step: set aside the baby&apos;s portion (no seasoning)
                    </p>
                  )}
                  <p>{s.text_en || s.text_cn}</p>
                  <p className="text-xs text-stone-400">{s.text_cn}</p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      )}
    </div>
  );
}

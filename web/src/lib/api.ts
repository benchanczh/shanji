const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost:8090";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

type Envelope<T> = { success: boolean; data?: T; error?: string };

export async function api<T>(
  method: string,
  path: string,
  body?: unknown,
  opts?: { noAuth?: boolean }
): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (!opts?.noAuth) {
    const token = typeof window !== "undefined" ? localStorage.getItem("token") : null;
    if (token) headers.Authorization = `Bearer ${token}`;
  }
  const res = await fetch(`${BASE}/api/v1${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const json = (await res.json().catch(() => ({ success: false, error: "服务器响应异常" }))) as Envelope<T>;
  if (!res.ok || !json.success) {
    if (res.status === 401 && !opts?.noAuth && typeof window !== "undefined" && !path.startsWith("/maid")) {
      localStorage.removeItem("token");
      window.location.href = "/login";
    }
    throw new ApiError(res.status, json.error ?? res.statusText);
  }
  return json.data as T;
}

// ---- shared types (mirror Go read models) ----

export type Household = {
  id: number;
  name: string;
  primary_cuisine: string | null;
  secondary_cuisine: string | null;
  cuisine_ratio: number;
  serving_factor: number;
};

export type DishView = {
  id: number;
  recipe_id: number;
  name: string;
  name_en: string;
  cuisine: string;
  course: string;
  target: string;
};

export type MealView = { slot_id: number; locked: boolean; dishes: DishView[] };
export type DayView = { date: string; meals: Record<string, MealView> };
export type PlanView = { id: number; week_start: string; status: string; days: DayView[] };

export type ShoppingItem = {
  id: number;
  name: string;
  name_en: string;
  total_qty: number | null;
  unit: string;
  category: string;
  checked: boolean;
};

export type Member = { id: number; name: string; age: number | null; role: string };

export type DietRule = {
  id: number;
  member_id: number | null;
  type: string;
  severity: string;
  ingredient_name: string | null;
  tag: string | null;
  note: string | null;
};

export type Suggestion = {
  id: number;
  from_role: string;
  title: string;
  content: string;
  status: string;
  created_at: string;
};

export type MaidStep = { order: number; text_cn: string; text_en: string; baby_split_point: boolean };
export type MaidIngredient = { name: string; name_en: string; qty: number | null; unit: string };
export type MaidDish = {
  recipe_id: number;
  name: string;
  name_en: string;
  course: string;
  target: string;
  minutes: number;
  steps: MaidStep[];
  ingredients: MaidIngredient[];
};
export type MaidDay = { date: string; meals: Record<string, MaidDish[]> };

// ---- date helpers ----

export function mondayOf(d: Date): string {
  const date = new Date(d);
  const day = (date.getDay() + 6) % 7; // Monday = 0
  date.setDate(date.getDate() - day);
  return date.toISOString().slice(0, 10);
}

export function addDays(iso: string, days: number): string {
  const d = new Date(iso + "T00:00:00");
  d.setDate(d.getDate() + days);
  return d.toISOString().slice(0, 10);
}

export const MEAL_LABELS: Record<string, string> = { breakfast: "早餐", lunch: "午餐", dinner: "晚餐" };
export const COURSE_LABELS: Record<string, string> = { main: "荤", side: "素", soup: "汤", breakfast: "早" };
export const CATEGORY_LABELS: Record<string, string> = {
  meat: "肉类",
  seafood: "海鲜",
  vegetable: "蔬菜",
  fruit: "水果",
  dairy: "蛋奶",
  staple: "主食",
  condiment: "调料",
  other: "其他",
};

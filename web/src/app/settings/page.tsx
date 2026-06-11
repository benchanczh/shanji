"use client";

import { useEffect, useState } from "react";
import { api, type DietRule, type Household, type Member } from "@/lib/api";

const CUISINES = ["粤菜", "川菜", "湘菜", "江浙菜", "东北菜", "家常"];
const ROLE_LABELS: Record<string, string> = { decider: "决策者", spouse: "配偶", child: "孩子", helper: "帮手" };
const RULE_TYPE_LABELS: Record<string, string> = { allergy: "过敏", forbidden: "忌口", baby: "宝宝", health: "健康", taste: "口味" };

export default function SettingsPage() {
  const [h, setH] = useState<Household | null>(null);
  const [members, setMembers] = useState<Member[]>([]);
  const [rules, setRules] = useState<DietRule[]>([]);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");

  const [newMember, setNewMember] = useState({ name: "", age: "", role: "child" });
  const [newRule, setNewRule] = useState({ type: "allergy", ingredient: "", member_id: "", note: "" });

  const loadAll = async () => {
    const [hh, ms, rs] = await Promise.all([
      api<Household>("GET", "/household"),
      api<Member[]>("GET", "/members"),
      api<DietRule[]>("GET", "/rules"),
    ]);
    setH(hh); setMembers(ms); setRules(rs);
  };

  useEffect(() => { loadAll().catch((e) => setError(e.message)); }, []);

  const run = async (fn: () => Promise<unknown>, ok?: string) => {
    setError(""); setMsg("");
    try {
      await fn();
      await loadAll();
      if (ok) { setMsg(ok); setTimeout(() => setMsg(""), 2500); }
    } catch (e) {
      setError(e instanceof Error ? e.message : "操作失败");
    }
  };

  if (!h) return <p className="text-stone-400">{error || "加载中…"}</p>;

  return (
    <div className="space-y-6">
      {error && <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>}
      {msg && <p className="rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{msg}</p>}

      <section className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm">
        <h2 className="mb-4 font-bold">口味偏好</h2>
        <div className="grid gap-4 sm:grid-cols-3">
          <label className="block text-sm">
            <span className="text-stone-500">主菜系</span>
            <select
              value={h.primary_cuisine ?? ""}
              onChange={(e) => setH({ ...h, primary_cuisine: e.target.value || null })}
              className="mt-1 w-full rounded-lg border border-stone-300 px-2 py-2"
            >
              <option value="">不设置</option>
              {CUISINES.map((c) => <option key={c}>{c}</option>)}
            </select>
          </label>
          <label className="block text-sm">
            <span className="text-stone-500">次菜系</span>
            <select
              value={h.secondary_cuisine ?? ""}
              onChange={(e) => setH({ ...h, secondary_cuisine: e.target.value || null })}
              className="mt-1 w-full rounded-lg border border-stone-300 px-2 py-2"
            >
              <option value="">不设置</option>
              {CUISINES.map((c) => <option key={c}>{c}</option>)}
            </select>
          </label>
          <label className="block text-sm">
            <span className="text-stone-500">主菜系占比 {h.cuisine_ratio}%</span>
            <input
              type="range" min={0} max={100} step={5}
              value={h.cuisine_ratio}
              onChange={(e) => setH({ ...h, cuisine_ratio: Number(e.target.value) })}
              className="mt-3 w-full accent-emerald-600"
            />
          </label>
        </div>
        <button
          onClick={() => run(() => api("PUT", "/household", {
            primary_cuisine: h.primary_cuisine,
            secondary_cuisine: h.secondary_cuisine,
            cuisine_ratio: h.cuisine_ratio,
          }), "已保存")}
          className="mt-4 rounded-lg bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-700"
        >
          保存
        </button>
      </section>

      <section className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm">
        <h2 className="mb-4 font-bold">家庭成员</h2>
        <ul className="mb-4 divide-y divide-stone-100">
          {members.map((m) => (
            <li key={m.id} className="flex items-center py-2 text-sm">
              <span className="font-medium">{m.name}</span>
              <span className="ml-2 text-stone-400">{ROLE_LABELS[m.role]}{m.age != null ? ` · ${m.age}岁` : ""}</span>
              <button onClick={() => run(() => api("DELETE", `/members/${m.id}`))} className="ml-auto text-xs text-stone-300 hover:text-red-500">删除</button>
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap gap-2">
          <input placeholder="姓名" value={newMember.name} onChange={(e) => setNewMember({ ...newMember, name: e.target.value })} className="w-28 rounded-lg border border-stone-300 px-2 py-1.5 text-sm" />
          <input placeholder="年龄" value={newMember.age} onChange={(e) => setNewMember({ ...newMember, age: e.target.value })} className="w-20 rounded-lg border border-stone-300 px-2 py-1.5 text-sm" />
          <select value={newMember.role} onChange={(e) => setNewMember({ ...newMember, role: e.target.value })} className="rounded-lg border border-stone-300 px-2 py-1.5 text-sm">
            {Object.entries(ROLE_LABELS).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
          <button
            disabled={!newMember.name}
            onClick={() => run(() => api("POST", "/members", {
              name: newMember.name,
              age: newMember.age ? Number(newMember.age) : null,
              role: newMember.role,
            }), "成员已添加").then(() => setNewMember({ name: "", age: "", role: "child" }))}
            className="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
          >
            添加
          </button>
        </div>
      </section>

      <section className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm">
        <h2 className="mb-1 font-bold">饮食规则</h2>
        <p className="mb-4 text-xs text-stone-400">过敏/忌口是硬规则——含该食材的菜不会出现在菜单里</p>
        <ul className="mb-4 divide-y divide-stone-100">
          {rules.map((r) => (
            <li key={r.id} className="flex items-center py-2 text-sm">
              <span className="rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-600">{RULE_TYPE_LABELS[r.type]}</span>
              <span className="ml-2 font-medium">{r.ingredient_name ?? r.tag}</span>
              {r.member_id && <span className="ml-2 text-xs text-stone-400">{members.find((m) => m.id === r.member_id)?.name}</span>}
              {r.note && <span className="ml-2 text-xs text-stone-400">{r.note}</span>}
              <button onClick={() => run(() => api("DELETE", `/rules/${r.id}`))} className="ml-auto text-xs text-stone-300 hover:text-red-500">删除</button>
            </li>
          ))}
          {rules.length === 0 && <li className="py-2 text-sm text-stone-400">暂无规则</li>}
        </ul>
        <div className="flex flex-wrap gap-2">
          <select value={newRule.type} onChange={(e) => setNewRule({ ...newRule, type: e.target.value })} className="rounded-lg border border-stone-300 px-2 py-1.5 text-sm">
            {Object.entries(RULE_TYPE_LABELS).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
          <input placeholder="食材名（如：虾仁）" value={newRule.ingredient} onChange={(e) => setNewRule({ ...newRule, ingredient: e.target.value })} className="w-40 rounded-lg border border-stone-300 px-2 py-1.5 text-sm" />
          <select value={newRule.member_id} onChange={(e) => setNewRule({ ...newRule, member_id: e.target.value })} className="rounded-lg border border-stone-300 px-2 py-1.5 text-sm">
            <option value="">全家</option>
            {members.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
          </select>
          <input placeholder="备注" value={newRule.note} onChange={(e) => setNewRule({ ...newRule, note: e.target.value })} className="w-32 rounded-lg border border-stone-300 px-2 py-1.5 text-sm" />
          <button
            disabled={!newRule.ingredient}
            onClick={() => run(() => api("POST", "/rules", {
              type: newRule.type,
              ingredient: newRule.ingredient,
              member_id: newRule.member_id ? Number(newRule.member_id) : null,
              note: newRule.note || null,
            }), "规则已添加").then(() => setNewRule({ type: "allergy", ingredient: "", member_id: "", note: "" }))}
            className="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
          >
            添加
          </button>
        </div>
      </section>

      <MaidLinkSection />
    </div>
  );
}

function MaidLinkSection() {
  const [link, setLink] = useState("");
  const [msg, setMsg] = useState("");

  const create = async () => {
    const data = await api<{ token: string }>("POST", "/maid-link");
    setLink(`${window.location.origin}/maid?token=${data.token}`);
    setMsg("");
  };
  const revoke = async () => {
    await api("DELETE", "/maid-link");
    setLink("");
    setMsg("已撤销全部链接");
  };

  return (
    <section className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm">
      <h2 className="mb-1 font-bold">阿姨链接</h2>
      <p className="mb-4 text-xs text-stone-400">免登录链接，阿姨打开即可看到今天做什么（英文界面）。生成新链接会让旧链接失效。</p>
      <div className="flex gap-2">
        <button onClick={create} className="rounded-lg bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-700">生成链接</button>
        <button onClick={revoke} className="rounded-lg border border-stone-300 px-4 py-1.5 text-sm hover:bg-stone-100">撤销全部</button>
      </div>
      {link && (
        <div className="mt-3 rounded-lg bg-stone-50 p-3">
          <p className="break-all font-mono text-xs text-stone-600">{link}</p>
          <button onClick={() => navigator.clipboard.writeText(link)} className="mt-2 text-xs text-emerald-600 hover:underline">复制链接（只显示这一次，请立即发给阿姨）</button>
        </div>
      )}
      {msg && <p className="mt-3 text-sm text-stone-500">{msg}</p>}
    </section>
  );
}

import { useEffect, useState } from "react";
import { api } from "../lib/api";
import type { ProfileTargets, UserProfile } from "../lib/types";
import { useT } from "../i18n";

// ФТ-01: профиль и норма калорий/БЖУ. Норму считает сервер (internal/nutrition) — здесь только форма
// и показ результата, чтобы формулы жили в одном месте и были покрыты тестами.

const ACTIVITIES = ["sedentary", "light", "moderate", "high", "extreme"] as const;
const GOALS = ["lose", "recomp", "maintain", "gain"] as const;
const PACES = ["slow", "normal", "fast"] as const;

const EMPTY: UserProfile = { sex: "m", birthDate: "", heightCm: 0, weightKg: 0, activity: "moderate", goal: "maintain", pace: "normal" };

export function ProfileCard() {
  const { t } = useT();
  const [form, setForm] = useState<UserProfile | null>(null);
  const [targets, setTargets] = useState<ProfileTargets | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.profile().then((v) => {
      setForm(v ? v.profile : EMPTY);
      setTargets(v && v.targets.kcal > 0 ? v.targets : null);
    }).catch((e: Error) => setError(e.message));
  }, []);

  if (!form) return error ? <p className="form-error">{error}</p> : <div className="skeleton" style={{ height: 240 }} />;

  const set = (patch: Partial<UserProfile>) => { setForm({ ...form, ...patch }); setDirty(true); setError(""); };
  const num = (v: string) => (v === "" ? 0 : Number(v.replace(",", ".")));
  const showPace = form.goal === "lose" || form.goal === "gain";

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      const v = await api.saveProfile(form);
      setForm(v.profile);
      setTargets(v.targets);
      setDirty(false);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="profile">
      {targets && <TargetsView tg={targets} />}

      <div className="profile__grid">
        <label className="profile__field">
          <span>{t("profile.sex")}</span>
          <div className="segmented segmented--2" role="radiogroup" aria-label={t("profile.sex")}>
            {(["m", "f"] as const).map((s) => (
              <button key={s} type="button" role="radio" aria-checked={form.sex === s} onClick={() => set({ sex: s })}>{t(`profile.sex.${s}`)}</button>
            ))}
          </div>
        </label>
        <label className="profile__field">
          <span>{t("profile.birth")}</span>
          <input className="form-control" type="date" value={form.birthDate} max={new Date().toISOString().slice(0, 10)} onChange={(e) => set({ birthDate: e.target.value })} />
        </label>
        <label className="profile__field">
          <span>{t("profile.height")}</span>
          <input className="form-control" type="number" inputMode="decimal" min={120} max={230} step={0.5} value={form.heightCm || ""} onChange={(e) => set({ heightCm: num(e.target.value) })} />
        </label>
        <label className="profile__field">
          <span>{t("profile.weight")}</span>
          <input className="form-control" type="number" inputMode="decimal" min={35} max={300} step={0.1} value={form.weightKg || ""} onChange={(e) => set({ weightKg: num(e.target.value) })} />
        </label>
        <label className="profile__field">
          <span>{t("profile.bodyfat")}</span>
          <input className="form-control" type="number" inputMode="decimal" min={3} max={60} step={0.1} value={form.bodyFatPct || ""} placeholder={t("profile.bodyfat.placeholder")} onChange={(e) => set({ bodyFatPct: num(e.target.value) })} />
        </label>
        <label className="profile__field">
          <span>{t("profile.activity")}</span>
          <select className="form-control" value={form.activity} onChange={(e) => set({ activity: e.target.value })}>
            {ACTIVITIES.map((a) => <option key={a} value={a}>{t(`profile.activity.${a}`)}</option>)}
          </select>
        </label>
      </div>

      <div className="profile__field">
        <span>{t("profile.goal")}</span>
        <div className="chips" role="radiogroup" aria-label={t("profile.goal")}>
          {GOALS.map((g) => (
            <button key={g} type="button" role="radio" className="chip chip--sm" aria-checked={form.goal === g} aria-pressed={form.goal === g} onClick={() => set({ goal: g })}>{t(`profile.goal.${g}`)}</button>
          ))}
        </div>
        <p className="quiz__hint">{t(`profile.goal.${form.goal}.hint`)}</p>
      </div>

      {showPace && (
        <div className="profile__field">
          <span>{t("profile.pace")}</span>
          <div className="segmented segmented--3" role="radiogroup" aria-label={t("profile.pace")}>
            {PACES.map((p) => (
              <button key={p} type="button" role="radio" aria-checked={form.pace === p} onClick={() => set({ pace: p })}>{t(`profile.pace.${p}`)}</button>
            ))}
          </div>
        </div>
      )}

      {error && <p className="form-error" role="alert">{error}</p>}
      {(dirty || !targets) && (
        <button type="button" className="btn btn-primary" onClick={save} disabled={saving}>
          {targets ? t("profile.save") : t("profile.calc")}
        </button>
      )}
      <p className="quiz__hint">{t("profile.disclaimer")}</p>
    </div>
  );
}

function TargetsView({ tg }: { tg: ProfileTargets }) {
  const { t } = useT();
  const fmt = (n: number) => n.toLocaleString("ru-RU");
  const weekly = tg.weeklyKg === 0 ? t("profile.weekly.zero") : t("profile.weekly", { kg: (tg.weeklyKg > 0 ? "+" : "") + tg.weeklyKg.toLocaleString("ru-RU") });
  return (
    <div className="targets" aria-live="polite">
      <div className="targets__kcal">
        <strong>{fmt(tg.kcal)}</strong> <span>{t("profile.kcalday")}</span>
      </div>
      <div className="targets__macros">
        <div><strong>{tg.proteinG}</strong><span>{t("profile.protein")}</span></div>
        <div><strong>{tg.fatG}</strong><span>{t("profile.fat")}</span></div>
        <div><strong>{tg.carbsG}</strong><span>{t("profile.carbs")}</span></div>
      </div>
      <p className="targets__meta">
        {t(`profile.method.${tg.method}`)} · {t("profile.rmr", { n: fmt(tg.rmr) })} · {t("profile.tdee", { n: fmt(tg.tdee) })}
        {tg.shiftPct !== 0 && <> · {t("profile.shift", { p: (tg.shiftPct > 0 ? "+" : "") + tg.shiftPct })}</>}
      </p>
      <p className="targets__meta">{weekly} · {t("profile.bmi", { n: tg.bmi.toLocaleString("ru-RU") })}{tg.leanMassKg ? <> · {t("profile.lean", { n: tg.leanMassKg.toLocaleString("ru-RU") })}</> : null}</p>
      {tg.notes?.map((n) => <p key={n} className="targets__note">{t(n)}</p>)}
    </div>
  );
}

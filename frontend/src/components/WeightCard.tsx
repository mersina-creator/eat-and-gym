import { useEffect, useMemo, useState } from "react";
import { api } from "../lib/api";
import type { WeightView } from "../lib/types";

// ФТ-05, шаг 1: журнал веса. Тренд и скорость считает сервер (internal/journal) — здесь только
// ввод и показ. Суточные качели на 1–2 кг от воды и соли сглажены, поэтому линия тренда
// важнее точек: по ней видно движение, по точкам — только шум.

const today = () => new Date().toLocaleDateString("sv"); // sv даёт ГГГГ-ММ-ДД в местной дате

export function WeightCard() {
  const [view, setView] = useState<WeightView | null>(null);
  const [value, setValue] = useState("");
  const [date, setDate] = useState(today());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [showImport, setShowImport] = useState(false);
  const [importText, setImportText] = useState("");

  const load = () => {
    api
      .weightJournal(14, today())
      .then(setView)
      .catch((e: Error) => setError(e.message));
  };

  useEffect(load, []);

  const save = async () => {
    const kg = Number(value.replace(",", "."));
    if (!kg) {
      setError("Введите вес");
      return;
    }
    setBusy(true);
    setError("");
    try {
      setView(await api.setWeight(date, kg, today()));
      setValue("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const runImport = async () => {
    setBusy(true);
    setError("");
    try {
      const res = await api.importWeight(importText, today());
      setView(res.view);
      setImportText("");
      setShowImport(false);
      if (res.result.errors?.length) setError(res.result.errors.join("; "));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // График: сетка, подписи шкалы в килограммах, точки замеров и линия тренда.
  // Шаг сетки подбирается под разброс: при ровном весе линии через 0,5 кг, при большом — через 5.
  const chart = useMemo(() => {
    const pts = view?.points ?? [];
    const vals = pts.flatMap((p) => [p.weight, p.trend]).filter((v): v is number => !!v);
    if (vals.length < 2) return null;

    const span = Math.max(...vals) - Math.min(...vals);
    const step = [0.5, 1, 2, 5, 10].find((s) => span / s <= 4) ?? 20;
    const lo = Math.floor((Math.min(...vals) - step / 2) / step) * step;
    const hi = Math.ceil((Math.max(...vals) + step / 2) / step) * step;

    const W = 320;
    const H = 96;
    const padR = 34;
    const padB = 14;
    const x = (i: number) => (pts.length < 2 ? 0 : (i / (pts.length - 1)) * (W - padR - 6) + 3);
    const y = (v: number) => (H - padB) - ((v - lo) / (hi - lo || 1)) * (H - padB - 8) - 4;

    const ticks: number[] = [];
    for (let v = lo; v <= hi + 1e-9; v += step) ticks.push(Number(v.toFixed(1)));

    const line = pts
      .map((p, i) => (p.trend ? `${i === 0 || !pts[i - 1].trend ? "M" : "L"}${x(i).toFixed(1)} ${y(p.trend).toFixed(1)}` : ""))
      .join("");
    const dots = pts
      .map((p, i) => (p.weight ? { cx: x(i), cy: y(p.weight), key: p.date } : null))
      .filter((d): d is { cx: number; cy: number; key: string } => !!d);

    const label = (iso: string) => `${iso.slice(8, 10)}.${iso.slice(5, 7)}`;
    const days = pts.length
      ? [
          { at: x(0), text: label(pts[0].date), anchor: "start" as const },
          { at: x(pts.length - 1), text: label(pts[pts.length - 1].date), anchor: "end" as const },
        ]
      : [];

    return { W, H, padR, padB, line, dots, ticks, y, days };
  }, [view]);

  const rate = view?.ratePerWeek;

  return (
    <div className="weight">
      <div className="weight__head">
        <h3>Вес</h3>
        {view?.last && (
          <span className="weight__last">
            Последний замер: {view.last.kg} кг, {view.last.date.slice(0, 10)}
          </span>
        )}
      </div>

      {chart && (
        <div className="weight__chart">
          <svg viewBox={`0 0 ${chart.W} ${chart.H}`} role="img" aria-label="График веса с линией тренда, килограммы">
            {chart.ticks.map((t) => (
              <g key={t}>
                <line x1="0" x2={chart.W - chart.padR} y1={chart.y(t)} y2={chart.y(t)} className="weight__grid" />
                <text x={chart.W - chart.padR + 5} y={chart.y(t) + 3.5} className="weight__tick">
                  {t}
                </text>
              </g>
            ))}
            <path d={chart.line} fill="none" stroke="currentColor" strokeWidth="2" className="weight__trend" />
            {chart.dots.map((d) => (
              <circle key={d.key} cx={d.cx} cy={d.cy} r="2.5" className="weight__dot" />
            ))}
            {chart.days.map((d) => (
              <text key={d.text} x={d.at} y={chart.H - 3} textAnchor={d.anchor} className="weight__tick">
                {d.text}
              </text>
            ))}
            <text x={chart.W - 2} y={9} textAnchor="end" className="weight__unit">
              кг
            </text>
          </svg>
        </div>
      )}

      {!!rate && (
        <p className="weight__rate">
          {rate > 0 ? "+" : "−"}
          {Math.abs(rate)} кг в неделю по тренду
        </p>
      )}

      <div className="weight__form">
        <input
          className="form-control"
          type="date"
          value={date}
          max={today()}
          onChange={(e) => setDate(e.target.value)}
        />
        <input
          className="form-control"
          type="number"
          inputMode="decimal"
          step="0.1"
          min="30"
          max="400"
          placeholder={view?.last ? String(view.last.kg) : "кг"}
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
        <button type="button" className="btn btn-primary" onClick={save} disabled={busy || !value}>
          Сохранить
        </button>
      </div>

      {error && <p className="form-error">{error}</p>}

      <p className="weight__cover">
        Замеров за 14 дней: {view?.coverage.weighIns ?? 0} из {view?.coverage.days ?? 14}
        {view && !view.ready && " — для пересчёта нормы пока мало"}
      </p>

      {!showImport && (
        <button type="button" className="btn btn-soft btn-sm" onClick={() => setShowImport(true)}>
          Загрузить историю
        </button>
      )}

      {showImport && (
        <div className="weight__import">
          <p className="quiz__hint">Построчно: дата и вес, например «2026-09-01 134,2».</p>
          <textarea
            className="form-control"
            rows={5}
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder={"2026-09-01 134,2\n2026-09-03 133,8"}
          />
          <div className="weight__form">
            <button type="button" className="btn btn-primary" onClick={runImport} disabled={busy || !importText.trim()}>
              Загрузить
            </button>
            <button type="button" className="btn btn-soft" onClick={() => setShowImport(false)}>
              Отмена
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

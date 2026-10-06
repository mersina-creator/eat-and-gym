import { useEffect, useState } from "react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { Activity, BookOpen, CalendarDays, ChefHat, Eye, Heart, Link2, LogOut, MessageCircle, Plus, ShoppingBag, ThumbsDown, ThumbsUp, Trash2, Upload, Users } from "lucide-react";
import { FamilyEditor } from "../components/FamilyEditor";
import { grownKids } from "../lib/kids";
import { SiteFooter } from "../components/SiteFooter";
import { TopBar } from "../components/TopBar";
import { OwnRecipeForm } from "../components/OwnRecipeForm";
import { BudgetChart } from "../components/BudgetChart";
import { PurchaseReceipts } from "../components/PurchaseReceipts";
import { CollectionsPanel } from "../components/CollectionsPanel";
import { TranslationLine } from "../components/TranslationStatus";
import { PhotoField } from "../components/PhotoField";
import { useConfirm } from "../components/Confirm";
import { NotifyCard } from "../components/NotifyCard";
import { InstallCard } from "../components/InstallSheet";
import { MessengerCard } from "../components/MessengerCard";
import { inTelegram } from "../lib/telegram";
import { EmptyState } from "../components/EmptyState";
import { api } from "../lib/api";
import { useAuth } from "../lib/auth";
import { approx, dateShort, weekRange } from "../lib/format";
import { track } from "../lib/analytics";
import type { Family, Favorite, Meta, OwnRecipe, PlanSummary, Purchase } from "../lib/types";
import { slotLabel } from "../lib/types";
import { langCountry, useT } from "../i18n";

type Tab = "plans" | "family" | "recipes" | "purchases";

export function Account() {
  const { user, admin, loading, setUser } = useAuth();
  const confirm = useConfirm();
  const nav = useNavigate();
  // вкладка в адресе (?tab=family): «назад» и обновление возвращают туда же
  const [sp, setSp] = useSearchParams();
  const TABS: Tab[] = ["plans", "family", "recipes", "purchases"];
  const tab: Tab = TABS.includes(sp.get("tab") as Tab) ? (sp.get("tab") as Tab) : "plans";
  const setTab = (next: Tab) => setSp((prev) => { const n = new URLSearchParams(prev); if (next === "plans") n.delete("tab"); else n.set("tab", next); return n; });
  const [plans, setPlans] = useState<PlanSummary[] | null>(null);
  const [purchases, setPurchases] = useState<Purchase[] | null>(null);
  const [dislikes, setDislikes] = useState<{ id: string; title: string; slot: string }[] | null>(null);
  const [own, setOwn] = useState<OwnRecipe[] | null>(null);
  const [editing, setEditing] = useState<OwnRecipe | "new" | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [name, setName] = useState(user?.name ?? "");
  const [nick, setNick] = useState(user?.nick ?? "");
  const [favorites, setFavorites] = useState<Favorite[] | null>(null);
  const [familyRaw, setFamilyRaw] = useState<Family | null>(null);
  // Go отдаёт slots без поля, когда едят всё: приводим к пустому списку
  const setFamily = (f: Family | null) => setFamilyRaw(f && { ...f, adults: f.adults.map((m) => ({ ...m, slots: m.slots ?? [] })), kids: grownKids(f.kids) }); // возраст детей растёт сам с отметки ageAt
  const family = familyRaw;
  const [familyDirty, setFamilyDirty] = useState(false);
  const [familySaving, setFamilySaving] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const { t, tn, lang } = useT();
  const approxRub = (v: number) => approx(v, undefined, lang);
  // Страна для цены своих рецептов: из последнего квиза, иначе по языку интерфейса.
  const countryCode = (user?.defaults as { country?: string } | undefined)?.country || langCountry(lang);
  const country = meta?.countries.find((c) => c.code === countryCode);

  useEffect(() => {
    if (!user) return;
    setName(user.name);
    setNick(user.nick ?? "");
    api.favorites().then(setFavorites).catch(() => setFavorites([]));
    // приглашение в семью по ссылке ?family=TOKEN
    const token = sp.get("family");
    if (token) {
      api.familyJoin(token).then((f) => { setFamily(f); setToast(t("account.family.joined")); }).catch((e: Error) => { setToast(e.message); api.family().then(setFamily).catch(() => setFamily(null)); });
      setSp({ tab: "family" }, { replace: true });
    } else {
      api.family().then(setFamily).catch(() => setFamily(null));
    }
    api.myPlans().then(setPlans).catch(() => setPlans([]));
    api.purchases(90).then(setPurchases).catch(() => setPurchases([]));
    api.dislikes().then(setDislikes).catch(() => setDislikes([]));
    api.meta(countryCode).then(setMeta).catch(() => setMeta(null));
    api.ownRecipes(countryCode).then(setOwn).catch(() => setOwn([]));
    track("account_view");
  }, [user?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!toast) return;
    // длинные подсказки (итог проверки уведомлений) держим дольше: примерно 60 мс на символ
    const t = window.setTimeout(() => setToast(null), Math.max(2000, Math.min(12000, toast.length * 60)));
    return () => window.clearTimeout(t);
  }, [toast]);


  if (loading) return null;
  if (!user) return <Navigate to="/login" replace />;

  const logout = async () => {
    await api.logout();
    setUser(null);
    nav("/");
  };

  const saveName = async () => {
    const u = await api.updateMe({ name });
    setUser(u);
    setToast(t("account.saved"));
  };
  const saveNick = async () => {
    try {
      const u = await api.updateMe({ nick });
      setUser(u);
      setNick(u.nick ?? "");
      setToast(t("account.saved"));
    } catch (e) {
      setToast((e as Error).message);
    }
  };
  const saveFamily = async () => {
    if (!family) return;
    setFamilySaving(true);
    try {
      setFamily(await api.saveFamily({ name: family.name, adults: family.adults, kids: family.kids }));
      setFamilyDirty(false);
      setToast(t("account.family.saved"));
    } catch (e) {
      setToast((e as Error).message);
    } finally {
      setFamilySaving(false);
    }
  };
  const inviteFamily = async () => {
    try {
      const { token } = await api.familyInvite();
      const url = `${window.location.origin}/me?family=${token}`;
      if (navigator.share) {
        await navigator.share({ title: t("account.family.invite.title"), url });
        return;
      }
      await navigator.clipboard.writeText(url);
      setToast(t("account.family.invite.copied"));
    } catch (e) {
      setToast((e as Error).message);
    }
  };
  const recipeHref = (id: string) => `${lang === "ru" ? "" : `/${lang}`}/recipe/${id}`;
  const shareOwn = async (r: OwnRecipe) => {
    const url = window.location.origin + recipeHref(r.id);
    try {
      if (r.status !== "approved") {
        const { status } = await api.publishOwn(r.id);
        setOwn((own ?? []).map((x) => (x.id === r.id ? { ...x, status: status as OwnRecipe["status"], public: status === "approved" } : x)));
        if (status !== "approved") setToast(t("own.publish.sent"));
      }
      if (navigator.share) {
        await navigator.share({ title: r.title, url });
        return;
      }
      await navigator.clipboard.writeText(url);
      setToast(t("social.copied"));
    } catch (e) {
      setToast((e as Error).message);
    }
  };

  return (
    <div className="shell shell--wide">
      <TopBar />
      <main className="account">
        <div className="account__head">
          <span className={"account__avatar" + (user.avatar ? " account__avatar--img" : "")} aria-hidden>
            {user.avatar ? <img src={user.avatar} alt="" /> : (user.name || user.email).slice(0, 1).toUpperCase()}
          </span>
          <div>
            <h1 className="account__title">{user.name ? t("account.hi", { name: user.name }) : t("account.title")}</h1>
            {/* у аккаунта из Telegram или VK без почты адрес служебный: человеку он ничего не скажет */}
            {!user.email.endsWith("@login.racion.app") && <p className="account__email">{user.email}</p>}
          </div>
        </div>
        <div className="segmented segmented--4" role="tablist" aria-label={t("account.tabs")}>
          <button type="button" role="tab" aria-selected={tab === "plans"} aria-checked={tab === "plans"} onClick={() => setTab("plans")}>
            <CalendarDays size={16} aria-hidden /> <span className="tab-long">{t("account.plans")}</span>
            <span className="tab-short">{t("account.plans.short")}</span>
          </button>
          <button type="button" role="tab" aria-selected={tab === "family"} aria-checked={tab === "family"} onClick={() => setTab("family")}>
            <Users size={16} aria-hidden /> <span className="tab-long">{t("account.family")}</span>
            <span className="tab-short">{t("account.family.short")}</span>
          </button>
          <button type="button" role="tab" aria-selected={tab === "recipes"} aria-checked={tab === "recipes"} onClick={() => setTab("recipes")}>
            <BookOpen size={16} aria-hidden /> <span className="tab-long">{t("account.recipes")}</span>
            <span className="tab-short">{t("account.recipes.short")}</span>
          </button>
          <button type="button" role="tab" aria-selected={tab === "purchases"} aria-checked={tab === "purchases"} onClick={() => setTab("purchases")}>
            <ShoppingBag size={16} aria-hidden /> <span className="tab-long">{t("account.purchases")}</span>
            <span className="tab-short">{t("account.purchases.short")}</span>
          </button>
        </div>

        {tab === "recipes" && (
          <section aria-label={t("account.recipes")} className="account__section">
            {editing ? (
              <OwnRecipeForm
                initial={editing === "new" ? null : editing}
                equipment={meta?.equipment ?? []}
                ai={meta?.ai}
                photos={meta?.photos}
                country={country}
                onSaved={(r) => {
                  setOwn(editing === "new" ? [r, ...(own ?? [])] : (own ?? []).map((x) => (x.id === r.id ? r : x)));
                  setEditing(null);
                  setToast(t("account.recipe.saved"));
                  track(editing === "new" ? "own_recipe_create" : "own_recipe_update");
                }}
                onCancel={() => setEditing(null)}
              />
            ) : (
              <>
                <p className="quiz__hint">{t("account.recipes.hint")}</p>
                {own === null && <div className="skeleton" style={{ height: 80 }} />}
                {own?.length === 0 && <EmptyState icon={<ChefHat size={20} />} text={t("account.recipes.empty")} />}
                {own?.map((r) => (
                  <div className="planrow" key={r.id}>
                    <button type="button" className="planrow__main planrow__main--btn" onClick={() => setEditing(r)} aria-label={t("account.recipe.edit", { title: r.title })}>
                      <span className="planrow__title">{r.title}</span>
                      <span className="planrow__meta">
                        {t("account.recipe.meta", { slot: slotLabel(lang, r.slot), time: r.timeMin, kcal: Math.round(r.kcal ?? 0), cost: approx(r.cost ?? 0, country, lang) })}
                      </span>
                      {r.status && r.status !== "private" && (
                        <span className={"planrow__status planrow__status--" + r.status}>{t(`own.status.${r.status}`, { note: r.note?.startsWith("auto-check") ? "" : (r.note ?? "") })}</span>
                      )}
                      {r.status === "approved" && (
                        <span className="planrow__stats" aria-label={t("own.stats")}>
                          <span title={t("own.stats.views")}><Eye size={13} aria-hidden /> {r.views ?? 0}</span>
                          <span title={t("own.stats.likes")}><ThumbsUp size={13} aria-hidden /> {r.likes ?? 0}</span>
                          <span title={t("own.stats.favorites")}><Heart size={13} aria-hidden /> {r.favorites ?? 0}</span>
                          <span title={t("own.stats.comments")}><MessageCircle size={13} aria-hidden /> {r.comments ?? 0}</span>
                        </span>
                      )}
                      {r.status === "improve" && r.suggestion && (
                        <span className="suggest-card" onClick={(e) => e.stopPropagation()} role="group" aria-label={t("own.suggest.title")}>
                          <b>{t("own.suggest.title")}</b>
                          {r.note && <small>{r.note}</small>}
                          <em>{r.suggestion.title}</em>
                          {r.suggestion.description && <span>{r.suggestion.description}</span>}
                          <ol>
                            {r.suggestion.steps.map((st, i) => (
                              <li key={i}>{st}</li>
                            ))}
                          </ol>
                          <span className="suggest-card__actions">
                            <button
                              type="button"
                              className="btn btn-primary btn-sm"
                              onClick={async (e) => {
                                e.stopPropagation();
                                try {
                                  const { status } = await api.suggestionOwn(r.id, true);
                                  setOwn((own ?? []).map((x) => (x.id === r.id ? { ...x, ...r.suggestion, status: status as OwnRecipe["status"], suggestion: undefined, public: status === "approved" } : x)));
                                  setToast(t("own.suggest.accepted"));
                                } catch (err) {
                                  setToast((err as Error).message);
                                }
                              }}
                            >
                              {t("own.suggest.accept")}
                            </button>
                            <button
                              type="button"
                              className="btn btn-soft btn-sm"
                              onClick={async (e) => {
                                e.stopPropagation();
                                try {
                                  const { status } = await api.suggestionOwn(r.id, false);
                                  setOwn((own ?? []).map((x) => (x.id === r.id ? { ...x, status: status as OwnRecipe["status"], suggestion: undefined } : x)));
                                  setToast(t("own.publish.sent"));
                                } catch (err) {
                                  setToast((err as Error).message);
                                }
                              }}
                            >
                              {t("own.suggest.keep")}
                            </button>
                          </span>
                        </span>
                      )}
                    </button>
                    <span className="planrow__actions">
                    {(!r.status || r.status === "private" || r.status === "rejected") && (
                      <button type="button" className="btn btn-soft btn-sm planrow__publish" onClick={() => shareOwn(r)} title={t("own.publish.hint")}>
                        <Upload size={15} aria-hidden /> {t("own.publish")}
                      </button>
                    )}
                    {r.status === "approved" && (
                      <button type="button" className="planrow__del planrow__share" aria-label={t("account.recipe.share")} title={t("account.recipe.public")} onClick={() => shareOwn(r)}>
                        <Link2 size={16} aria-hidden />
                      </button>
                    )}
                    <button
                      type="button"
                      className="planrow__del"
                      aria-label={t("account.recipe.delete")}
                      onClick={async () => {
                        if (!(await confirm({ title: t("account.recipe.confirm.title", { title: r.title }), text: t("account.recipe.confirm"), ok: t("delete"), danger: true }))) return;
                        await api.deleteOwnRecipe(r.id);
                        setOwn((own ?? []).filter((x) => x.id !== r.id));
                        setToast(t("account.recipe.deleted"));
                      }}
                    >
                      <Trash2 size={16} aria-hidden />
                    </button>
                    </span>
                    <TranslationLine recipeId={r.id} summary={r.translations} onToast={setToast} />
                  </div>
                ))}
                <button type="button" className="btn btn-primary" onClick={() => setEditing("new")}>
                  <Plus size={16} aria-hidden /> {t("account.recipes.add")}
                </button>
            <CollectionsPanel onToast={setToast} />
            <h3 className="account__sub">{t("account.favs")}</h3>
            <p className="quiz__hint">{t("account.favs.hint")}</p>
            {favorites === null && <div className="skeleton" style={{ height: 80 }} />}
            {favorites?.length === 0 && <EmptyState icon={<Heart size={20} />} text={t("account.favs.empty")} action={<Link className="btn btn-soft" to="/recipes">{t("nav.recipes")}</Link>} />}
            {favorites?.map((f) => (
              <div className="planrow" key={f.id}>
                <a className="planrow__main" href={recipeHref(f.id)}>
                  <span className="planrow__title">
                    {f.title}
                    {f.own && <span className="dish__own">{t("own.badge")}</span>}
                  </span>
                  <span className="planrow__meta">{slotLabel(lang, f.slot)}</span>
                </a>
                <button
                  type="button"
                  className="planrow__del"
                  aria-label={t("social.unfav")}
                  onClick={async () => {
                    await api.favorite(f.id, false);
                    setFavorites(favorites.filter((x) => x.id !== f.id));
                    setToast(t("social.unfav.done"));
                  }}
                >
                  <Trash2 size={16} aria-hidden />
                </button>
              </div>
            ))}
            <h3 className="account__sub">{t("account.dislikes")}</h3>
            <p className="quiz__hint">{t("account.dislikes.hint")}</p>
            {dislikes === null && <div className="skeleton" style={{ height: 80 }} />}
            {dislikes?.length === 0 && <EmptyState icon={<ThumbsDown size={20} />} text={t("account.dislikes.empty")} />}
            {dislikes?.map((d) => (
              <div className="planrow" key={d.id}>
                <a className="planrow__main" href={`${lang === "ru" ? "" : `/${lang}`}/recipe/${d.id}`}>
                  <span className="planrow__title">{d.title}</span>
                  <span className="planrow__meta">{slotLabel(lang, d.slot)}</span>
                </a>
                <button
                  type="button"
                  className="planrow__del"
                  aria-label={t("account.dislike.restore")}
                  onClick={async () => {
                    await api.undislike(d.id);
                    setDislikes(dislikes.filter((x) => x.id !== d.id));
                    setToast(t("account.restored"));
                  }}
                >
                  <Trash2 size={16} aria-hidden />
                </button>
              </div>
            ))}
              </>
            )}
          </section>
        )}

        {tab === "family" && (
          <section aria-label={t("account.family")} className="account__section">
            <p className="quiz__hint">{t("account.family.hint")}</p>
            {family === null && <div className="skeleton" style={{ height: 120 }} />}
            {family && (
              <>
                <FamilyEditor adults={family.adults} kids={family.kids} meta={meta} onChange={(next) => { setFamily({ ...family, ...next }); setFamilyDirty(true); }} />
                {familyDirty && (
                  <button type="button" className="btn btn-primary" onClick={saveFamily} disabled={familySaving}>
                    {t("account.family.save")}
                  </button>
                )}
                <h3 className="account__sub">{t("account.family.accounts")}</h3>
                <p className="quiz__hint">{t("account.family.accounts.hint")}</p>
                {family.accounts.map((a) => (
                  <div className="planrow" key={a.userId}>
                    <div className="planrow__main">
                      <span className="planrow__title">
                        {a.name}
                        {a.you && <span className="dish__own">{t("account.family.you")}</span>}
                      </span>
                      <span className="planrow__meta">
                        {a.nick ? `@${a.nick}` : ""}
                        {a.owner && <> {a.nick ? "· " : ""}{t("account.family.owner")}</>}
                      </span>
                    </div>
                    {family.owner && !a.you && (
                      <button type="button" className="planrow__del" aria-label={t("account.family.remove")} onClick={async () => { await api.familyRemove(a.userId); setFamily({ ...family, accounts: family.accounts.filter((x) => x.userId !== a.userId) }); }}>
                        <Trash2 size={16} aria-hidden />
                      </button>
                    )}
                  </div>
                ))}
                {family.accounts.length === 0 && <EmptyState icon={<Users size={20} />} text={t("account.family.accounts.empty")} />}
                <div className="account__actions">
                  {(family.owner || family.accounts.length === 0) && (
                    <button type="button" className="btn btn-soft" onClick={inviteFamily}>
                      <Link2 size={16} aria-hidden /> {t("account.family.invite")}
                    </button>
                  )}
                  {!family.owner && family.id && (
                    <button type="button" className="btn btn-ghost" onClick={async () => { await api.familyLeave(); setFamily(await api.family()); setToast(t("account.family.left")); }}>
                      {t("account.family.leave")}
                    </button>
                  )}
                </div>
              </>
            )}
          </section>
        )}

        {tab === "plans" && (
          <section aria-label={t("account.plans.aria")} className="account__section">
            {plans === null && <div className="skeleton" style={{ height: 120 }} />}
            {plans?.length === 0 && (
              <EmptyState icon={<CalendarDays size={20} />} text={t("account.plans.empty")} hint={t("account.plans.empty.hint")} action={<Link to="/" className="btn btn-primary">{t("account.plans.first")}</Link>} />
            )}
            {plans?.map((p) => (
              <div className="planrow" key={p.id}>
                <Link to={`/plan/${p.id}`} className="planrow__main">
                  <span className="planrow__title">
                    {p.title || (p.occasion ? `${p.occasion.title} ${t("occ.for", { n: p.occasion.guests, guests: tn("guests", p.occasion.guests) })}` : t("plan.title", { range: weekRange(p.startDate, lang) }))}
                    {p.shared && <span className="dish__own">{t("plan.shared")}</span>}
                  </span>
                  <span className="planrow__meta">
                    {p.occasion ? t("account.plan.meta.occ", { date: dateShort(p.date || p.startDate, lang), store: p.store, cost: approx(p.cost, p.country, lang) }) : t("account.plan.meta", { store: p.store, portions: p.portions, cost: approx(p.cost, p.country, lang) })}
                    {p.items > 0 && <> {t("account.plan.bought", { n: p.checked, total: p.items })}</>}
                  </span>
                </Link>
                <button
                  type="button"
                  className="planrow__del"
                  aria-label={t("account.plan.delete")}
                  onClick={async () => {
                    if (!(await confirm({ title: t("account.plan.confirm"), text: t("account.plan.confirm.text"), ok: t("delete"), danger: true }))) return;
                    await api.deletePlan(p.id);
                    setPlans(plans.filter((x) => x.id !== p.id));
                  }}
                >
                  <Trash2 size={16} aria-hidden />
                </button>
              </div>
            ))}
            {plans && plans.length > 0 && (
              <Link to="/" className="btn btn-soft">
                {t("account.plans.new")}
              </Link>
            )}
          </section>
        )}

        {tab === "purchases" && (
          <section aria-label={t("account.purchases")} className="account__section">
            <BudgetChart country={country} />
            <p className="quiz__hint purch__hint">{t("account.purchases.hint")}</p>
            {purchases === null && <div className="skeleton" style={{ height: 120 }} />}
            {purchases?.length === 0 && <EmptyState icon={<ShoppingBag size={20} />} text={t("account.purchases.empty")} />}
            {purchases && purchases.length > 0 && <PurchaseReceipts purchases={purchases} money={approxRub} />}
          </section>
        )}


        <section className="account__profile" aria-label={t("account.profile")}>
          <div className="account__card">
            <label className="auth__field">
              <span>{t("account.name")}</span>
              <div className="account__name">
                <input className="form-control" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} />
                <button type="button" className="btn btn-secondary" onClick={saveName} disabled={name === user.name}>
                  {t("save")}
                </button>
              </div>
            </label>
            {meta?.photos && (
              <div className="auth__field">
                <span>{t("photo.avatar")}</span>
                <PhotoField
                  value={user.avatar ?? ""}
                  kind="avatar"
                  round
                  onChange={(url) => {
                    api.updateMe({ avatar: url }).then((u) => setUser(u)).catch((e: Error) => setToast(e.message));
                  }}
                />
              </div>
            )}
            <label className="auth__field">
              <span>{t("account.nick")}</span>
              <div className="account__name">
                <input className="form-control" value={nick} onChange={(e) => setNick(e.target.value.toLowerCase())} maxLength={24} placeholder="vadim_k" autoComplete="off" spellCheck={false} />
                <button type="button" className="btn btn-secondary" onClick={saveNick} disabled={nick === (user.nick ?? "")}>
                  {t("save")}
                </button>
              </div>
              <small>{t("account.nick.hint")}</small>
            </label>
            <p className="quiz__hint">{t("account.remember")}</p>
            {admin && (
              <Link to="/admin" className="btn btn-soft">
                <Activity size={16} aria-hidden /> {t("admin.title")}
              </Link>
            )}
            <button type="button" className="btn btn-ghost" onClick={logout}>
              <LogOut size={16} aria-hidden /> {t("account.logout")}
            </button>
          </div>
          <MessengerCard onToast={setToast} />
          <NotifyCard onToast={setToast} />
          {!inTelegram() && <InstallCard />}
        </section>
      </main>
      <SiteFooter />
      {toast && (
        <div className="toast" role="status">
          {toast}
        </div>
      )}
    </div>
  );
}

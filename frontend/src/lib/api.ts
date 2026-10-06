import type { AdminError, AdminLog, AdminOverview, AdminRecipe, AdminUser, Collection, OccasionView, SubRow, CatalogRecipeInput, ModerationItem, BudgetReport, Child, Comment, Extra, Family, Favorite, Member, IngredientRef, Meta, NotifySettings, RecipeStats, OwnRecipe, OwnRecipeInput, Params, Plan, PlanSummary, Purchase, Recipe, StoreCart, User, TranslationStatus, Partner, PartnerView, ApiKey, Offer, ProfileTargets, UserProfile, WeightView } from "./types";
import { getLang, tStatic } from "../i18n";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// Язык страницы важнее языка браузера: на /en посетитель с немецким браузером должен получать
// английские названия стран и блюд, поэтому у каждого запроса к API язык указан явно.
function withLang(path: string): string {
  if (!path.startsWith("/api/") || /[?&]lang=/.test(path)) return path;
  return `${path}${path.includes("?") ? "&" : "?"}lang=${getLang()}`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(withLang(path), { ...init, headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) } });
  } catch {
    throw new ApiError(0, tStatic("api.offline"));
  }
  if (!res.ok) {
    let msg = tStatic("api.error", { status: res.status });
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      // тело не JSON — оставляем статус
    }
    if (res.status === 404) msg = tStatic("api.notfound");
    if (res.status >= 500) msg = tStatic("api.server");
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  meta: (country?: string) => request<Meta>(`/api/meta${country ? `?country=${country}` : ""}`),
  createPlan: (params: Params) => request<Plan>("/api/plans", { method: "POST", body: JSON.stringify(params) }),
  planChat: (id: string, message: string) => request<{ reply: string; applied: string[]; plan: Plan; model?: string }>(`/api/plans/${encodeURIComponent(id)}/chat`, { method: "POST", body: JSON.stringify({ message }) }),
  getPlan: (id: string, lang?: string) => request<Plan>(`/api/plans/${encodeURIComponent(id)}${lang ? `?lang=${encodeURIComponent(lang)}` : ""}`),
  swap: (id: string, day: number, slot: string) =>
    request<Plan>(`/api/plans/${encodeURIComponent(id)}/swap`, { method: "POST", body: JSON.stringify({ day, slot }) }),
  swapSide: (id: string, day: number, slot: string) =>
    request<Plan>(`/api/plans/${encodeURIComponent(id)}/side`, { method: "POST", body: JSON.stringify({ day, slot }) }),
  skipDay: (id: string, day: number, skip: boolean) => request<Plan>(`/api/plans/${encodeURIComponent(id)}/skip`, { method: "POST", body: JSON.stringify({ day, skip }) }),
  moveDish: (id: string, from: number, to: number, slot: string) => request<Plan>(`/api/plans/${encodeURIComponent(id)}/move`, { method: "POST", body: JSON.stringify({ from, to, slot }) }),
  // корзина сети одной ссылкой (ВкусВилл): ids — продукты, которые ещё не куплены
  storeCart: (id: string, ids: string[]) => request<StoreCart>(`/api/plans/${encodeURIComponent(id)}/cart`, { method: "POST", body: JSON.stringify({ ids }) }),
  repeatPlan: (id: string, startDate?: string) => request<Plan>(`/api/plans/${encodeURIComponent(id)}/repeat`, { method: "POST", body: JSON.stringify({ startDate: startDate ?? "" }) }),
  recipe: (id: string, country?: string) => request<Recipe>(`/api/recipes/${encodeURIComponent(id)}${country ? `?country=${encodeURIComponent(country)}` : ""}`),
  // аккаунт
  me: () => request<{ user: User | null; admin?: boolean; perms?: string[]; role?: string; links?: string[] }>("/api/me"),
  // мини-приложение в мессенджере: вход по подписи мессенджера; create — человек сам нажал «Войти».
  // tz — часовой пояс браузера: по нему бот присылает напоминания вовремя
  webAppAuth: (platform: string, initData: string, create?: boolean, plan?: string) =>
    request<{ status: "login" | "created" | "linked" | "other" | "guest"; user: User | null; open?: string }>(`/api/webapp/${platform}/auth`, {
      method: "POST",
      body: JSON.stringify({ initData, create: !!create, plan: plan ?? "", tz: -new Date().getTimezoneOffset() }),
    }),
  // «Список в Telegram» внутри мини-приложения: бот присылает список прямо в чат (409 — чата с ботом ещё нет)
  webAppList: (platform: string, initData: string, plan: string) => request<{ sent: boolean }>(`/api/webapp/${platform}/list`, { method: "POST", body: JSON.stringify({ initData, plan }) }),
  messengerLink: (platform: string) => request<{ url: string }>(`/api/me/messengers/${platform}`, { method: "POST" }),
  messengerUnlink: (platform: string) => request<{ ok: boolean }>(`/api/me/messengers/${platform}`, { method: "DELETE" }),
  adminRecipes: (q: string) => request<{ items: AdminRecipe[]; tags: string[] }>(`/api/admin/recipes?q=${encodeURIComponent(q)}`),
  adminRecipe: (id: string) => request<AdminRecipe>(`/api/admin/recipes/${encodeURIComponent(id)}`),
  adminSaveRecipe: (body: CatalogRecipeInput) => request<AdminRecipe>("/api/admin/recipes", { method: "POST", body: JSON.stringify(body) }),
  adminDeleteRecipe: (id: string) => request<void>(`/api/admin/recipes/${encodeURIComponent(id)}`, { method: "DELETE" }),
  adminPublishRecipe: (id: string, publish: boolean) => request<{ id: string; hidden: boolean }>(`/api/admin/recipes/${encodeURIComponent(id)}/publish`, { method: publish ? "POST" : "DELETE" }),
  adminModeration: () => request<{ queue: ModerationItem[]; recent: ModerationItem[] }>("/api/admin/moderation"),
  adminDecide: (id: string, approve: boolean, note: string) => request<void>(`/api/admin/moderation/${encodeURIComponent(id)}`, { method: "POST", body: JSON.stringify({ approve, note }) }),
  adminSetRole: (id: string, role: string) => request<void>(`/api/admin/users/${encodeURIComponent(id)}/role`, { method: "PUT", body: JSON.stringify({ role }) }),
  suggestionOwn: (id: string, accept: boolean) => request<{ status: string }>(`/api/me/recipes/${encodeURIComponent(id)}/suggestion`, { method: "POST", body: JSON.stringify({ accept }) }),
  ownTranslations: (id: string) => request<TranslationStatus>(`/api/me/recipes/${encodeURIComponent(id)}/translations`),
  ownTranslate: (id: string) => request<TranslationStatus>(`/api/me/recipes/${encodeURIComponent(id)}/translations`, { method: "POST" }),
  publishOwn: (id: string) => request<{ status: string }>(`/api/me/recipes/${encodeURIComponent(id)}/publish`, { method: "POST" }),
  adminAI: () => request<{ providers: { name: string; model: string; today: number; perDay: number; ok: number; failed: number; resting: boolean; restUntil?: string; lastError?: string }[] }>("/api/admin/ai"),
  adminOverview: (days: number) => request<AdminOverview>(`/api/admin/overview?days=${days}`),
  adminUsers: () => request<AdminUser[]>("/api/admin/users"),
  adminErrors: () => request<AdminError[]>("/api/admin/errors"),
  adminLogs: (level: string) => request<AdminLog[]>(`/api/admin/logs?level=${level}&n=300`),
  register: (email: string, password: string, name: string, plan?: string) =>
    request<User>(`/api/auth/register${plan ? `?plan=${plan}` : ""}`, { method: "POST", body: JSON.stringify({ email, password, name }) }),
  forgot: (email: string) => request<void>("/api/auth/forgot", { method: "POST", body: JSON.stringify({ email }) }),
  reset: (token: string, password: string, plan?: string) => request<User>("/api/auth/reset" + (plan ? `?plan=${plan}` : ""), { method: "POST", body: JSON.stringify({ token, password }) }),
  login: (email: string, password: string, plan?: string) =>
    request<User>(`/api/auth/login${plan ? `?plan=${plan}` : ""}`, { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => request<void>("/api/auth/logout", { method: "POST" }),
  // фото: multipart без Content-Type в заголовке (браузер поставит boundary сам)
  upload: async (kind: "recipe" | "comment" | "avatar" | "offer", file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    let res: Response;
    try {
      res = await fetch(`/api/uploads?kind=${kind}`, { method: "POST", body: fd });
    } catch {
      throw new ApiError(0, tStatic("api.offline"));
    }
    if (!res.ok) {
      let msg = tStatic("api.error", { status: res.status });
      try {
        const body = (await res.json()) as { error?: string };
        if (body.error) msg = body.error;
      } catch {
        // без тела
      }
      throw new ApiError(res.status, msg);
    }
    return (await res.json()) as { url: string; thumb: string; w: number; h: number };
  },
  updateMe: (body: { name?: string; nick?: string; avatar?: string; defaults?: unknown }) => request<User>("/api/me", { method: "PATCH", body: JSON.stringify(body) }),
  myPlans: () => request<PlanSummary[]>("/api/me/plans"),
  renamePlan: (id: string, title: string) => request<void>(`/api/plans/${id}`, { method: "PATCH", body: JSON.stringify({ title }) }),
  deletePlan: (id: string) => request<void>(`/api/plans/${id}`, { method: "DELETE" }),
  dislikes: () => request<{ id: string; title: string; slot: string }[]>("/api/me/dislikes"),
  dislike: (recipeId: string) => request<void>(`/api/me/dislikes/${encodeURIComponent(recipeId)}`, { method: "PUT" }),
  undislike: (recipeId: string) => request<void>(`/api/me/dislikes/${encodeURIComponent(recipeId)}`, { method: "DELETE" }),
  purchases: (days = 60) => request<Purchase[]>(`/api/me/purchases?days=${days}`),
  // свои рецепты
  ingredients: () => request<IngredientRef[]>("/api/ingredients"),
  ownRecipes: (country?: string) => request<OwnRecipe[]>(`/api/me/recipes${country ? `?country=${country}` : ""}`),
  createOwnRecipe: (body: OwnRecipeInput, country?: string) =>
    request<OwnRecipe>(`/api/me/recipes${country ? `?country=${country}` : ""}`, { method: "POST", body: JSON.stringify(body) }),
  updateOwnRecipe: (id: string, body: OwnRecipeInput, country?: string) =>
    request<OwnRecipe>(`/api/me/recipes/${encodeURIComponent(id)}${country ? `?country=${country}` : ""}`, { method: "PUT", body: JSON.stringify(body) }),
  budget: () => request<BudgetReport>("/api/me/budget"),
  collections: () => request<Collection[]>("/api/me/collections"),
  collectionCreate: (name: string) => request<Collection>("/api/me/collections", { method: "POST", body: JSON.stringify({ name }) }),
  collectionRename: (id: string, name: string) => request<void>(`/api/me/collections/${id}`, { method: "PUT", body: JSON.stringify({ name }) }),
  collectionDelete: (id: string) => request<void>(`/api/me/collections/${id}`, { method: "DELETE" }),
  collectionItems: (id: string) => request<Favorite[]>(`/api/me/collections/${id}/items`),
  collectionToggle: (id: string, recipeId: string, on: boolean) => request<void>(`/api/me/collections/${id}/items/${encodeURIComponent(recipeId)}`, { method: on ? "PUT" : "DELETE" }),
  collectionPublish: (id: string, pub: boolean) => request<Collection>(`/api/me/collections/${id}/public`, { method: "PUT", body: JSON.stringify({ public: pub }) }),
  publicCollections: (lang: string) => request<Collection[]>(`/api/collections?lang=${encodeURIComponent(lang)}`),
  // подвал: только подборки из серверного подвала — имя и адрес, а не весь список под мегабайт
  featuredCollections: (lang: string) => request<{ slug: string; name: string }[]>(`/api/collections/featured?lang=${encodeURIComponent(lang)}`),
  partners: (country: string) => request<PartnerView>(`/api/partners?country=${encodeURIComponent(country)}`),
  apiKeys: () => request<ApiKey[]>("/api/me/keys"),
  apiKeyCreate: (name: string) => request<ApiKey>("/api/me/keys", { method: "POST", body: JSON.stringify({ name }) }),
  apiKeyDelete: (id: string) => request<void>(`/api/me/keys/${encodeURIComponent(id)}`, { method: "DELETE" }),
  offers: (place: "cart" | "recipe" | "plan", country: string, region?: string, match?: string[]) => {
    const q = new URLSearchParams({ place, country });
    if (region) q.set("region", region);
    if (match?.length) q.set("match", match.join(","));
    return request<Offer[]>(`/api/offers?${q}`);
  },
  adminAds: () => request<{ enabled: boolean }>("/api/admin/ads"),
  adminSetAds: (enabled: boolean) => request<{ enabled: boolean }>("/api/admin/ads", { method: "PUT", body: JSON.stringify({ enabled }) }),
  adminOffers: () => request<Offer[]>("/api/admin/offers"),
  adminSaveOffer: (body: Offer) => request<Offer>("/api/admin/offers", { method: "POST", body: JSON.stringify(body) }),
  adminDeleteOffer: (id: string) => request<void>(`/api/admin/offers/${encodeURIComponent(id)}`, { method: "DELETE" }),
  adminPartners: () => request<Partner[]>("/api/admin/partners"),
  adminSavePartner: (body: Partner) => request<Partner>("/api/admin/partners", { method: "POST", body: JSON.stringify(body) }),
  adminDeletePartner: (code: string) => request<void>(`/api/admin/partners/${encodeURIComponent(code)}`, { method: "DELETE" }),
  adminCollections: () => request<Collection[]>("/api/admin/collections"),
  adminSaveCollection: (body: { id: string; name: string; slug: string; description: string; cover: string; public: boolean; recipes: string[]; names: Record<string, string>; descriptions: Record<string, string> }) => request<Collection>("/api/admin/collections", { method: "POST", body: JSON.stringify(body) }),
  adminDeleteCollection: (id: string) => request<void>(`/api/admin/collections/${id}`, { method: "DELETE" }),
  // язык в адресе: ответ кэшируется браузером на 10 минут, и без него после смены языка приходили старые названия
  occasions: (lang: string) => request<OccasionView[]>(`/api/occasions?lang=${encodeURIComponent(lang)}`),
  createOccasion: (id: string, guests: number, params: Params) => request<Plan>(`/api/occasions/${id}`, { method: "POST", body: JSON.stringify({ guests, params }) }),
  createTable: (items: { recipeId: string; servings: number }[], guests: number, params: Params) => request<Plan>("/api/baskets", { method: "POST", body: JSON.stringify({ items, guests, params }) }),
  recipeSubs: (id: string, country?: string) => request<SubRow[]>(`/api/recipes/${encodeURIComponent(id)}/subs${country ? `?country=${country}` : ""}`),
  feedback: (id: string, liked: boolean) => request<{ ok: boolean }>(`/api/recipes/${encodeURIComponent(id)}/feedback`, { method: "POST", body: JSON.stringify({ liked }) }),
  deleteOwnRecipe: (id: string) => request<void>(`/api/me/recipes/${encodeURIComponent(id)}`, { method: "DELETE" }),
  // помощник: поправить текст (improve) или перевести (translate) — ничего не сохраняет
  assistRecipe: (body: { action: "improve" | "translate"; lang: string; title: string; description: string; steps: string[] }) =>
    request<{ title: string; description: string; steps: string[] }>("/api/me/recipes/ai", { method: "POST", body: JSON.stringify(body) }),
  // лайки, избранное, комментарии
  like: (id: string, on: boolean) => request<RecipeStats>(`/api/recipes/${encodeURIComponent(id)}/like`, { method: on ? "PUT" : "DELETE" }),
  favorite: (id: string, on: boolean) => request<RecipeStats>(`/api/recipes/${encodeURIComponent(id)}/favorite`, { method: on ? "PUT" : "DELETE" }),
  comments: (id: string) => request<Comment[]>(`/api/recipes/${encodeURIComponent(id)}/comments`),
  addComment: (id: string, body: string) => request<Comment>(`/api/recipes/${encodeURIComponent(id)}/comments`, { method: "POST", body: JSON.stringify({ body }) }),
  deleteComment: (id: number) => request<void>(`/api/comments/${id}`, { method: "DELETE" }),
  favorites: () => request<Favorite[]>("/api/me/favorites"),
  setOwnPublic: (id: string, pub: boolean) => request<{ status: string }>(`/api/me/recipes/${encodeURIComponent(id)}/public`, { method: "PUT", body: JSON.stringify({ public: pub }) }),
  // семья (состав и аккаунты)
  family: () => request<Family>("/api/me/family"),
  saveFamily: (body: { name: string; adults: Member[]; kids: Child[] }) => request<Family>("/api/me/family", { method: "PUT", body: JSON.stringify(body) }),
  familyInvite: (reset = false) => request<{ token: string }>(`/api/me/family/invite${reset ? "?reset=1" : ""}`, { method: "POST" }),
  familyJoin: (token: string) => request<Family>("/api/me/family/join", { method: "POST", body: JSON.stringify({ token }) }),
  familyLeave: () => request<void>("/api/me/family/leave", { method: "POST" }),
  familyRemove: (userId: string) => request<void>(`/api/me/family/accounts/${userId}`, { method: "DELETE" }),
  // семья и уведомления
  joinPlan: (id: string) => request<void>(`/api/plans/${encodeURIComponent(id)}/join`, { method: "POST" }),
  pushKey: () => request<{ key: string }>("/api/push/key"),
  pushSubscribe: (body: { endpoint: string; p256dh: string; auth: string }) => request<void>("/api/me/push", { method: "POST", body: JSON.stringify(body) }),
  pushUnsubscribe: (endpoint: string) => request<void>("/api/me/push", { method: "DELETE", body: JSON.stringify({ endpoint }) }),
  // ФТ-01: профиль и норма калорий/БЖУ; null — профиль ещё не заполнен
  profile: () => request<{ profile: UserProfile; targets: ProfileTargets } | null>("/api/me/profile"),
  saveProfile: (p: UserProfile) => request<{ profile: UserProfile; targets: ProfileTargets }>("/api/me/profile", { method: "PUT", body: JSON.stringify(p) }),
  // ФТ-05: журнал веса
  weightJournal: (days: number, today: string) => request<WeightView>(`/api/me/journal/weight?days=${days}&today=${today}`),
  setWeight: (date: string, kg: number, today: string) =>
    request<WeightView>(`/api/me/weight/${date}`, { method: "PUT", body: JSON.stringify({ kg, today }) }),
  deleteWeight: (date: string, today: string) =>
    request<WeightView>(`/api/me/weight/${date}?today=${today}`, { method: "DELETE" }),
  importWeight: (text: string, today: string) =>
    request<{ result: { added: number; errors?: string[] }; view: WeightView }>("/api/me/weight/import", { method: "POST", body: JSON.stringify({ text, today }) }),
  notify: () => request<{ settings: NotifySettings; devices: number }>("/api/me/notify"),
  setNotify: (s: NotifySettings) => request<void>("/api/me/notify", { method: "PUT", body: JSON.stringify(s) }),
  notifyTest: () => request<void>("/api/me/notify/test", { method: "POST" }),
  notifyTestResult: (result: string) => request<void>("/api/me/notify/test/result", { method: "POST", body: JSON.stringify({ result, ua: navigator.userAgent }) }),
  // список: отметки и свои товары
  checks: (planId: string) => request<string[]>(`/api/plans/${planId}/checks`),
  setCheck: (planId: string, body: { itemId: string; checked: boolean; name?: string; qty?: string; cost?: number }) =>
    request<void>(`/api/plans/${planId}/checks`, { method: "PUT", body: JSON.stringify(body) }),
  extras: (planId: string) => request<Extra[]>(`/api/plans/${planId}/extras`),
  addExtra: (planId: string, body: { name: string; qty?: string; due?: string | null; note?: string }) =>
    request<Extra>(`/api/plans/${planId}/extras`, { method: "POST", body: JSON.stringify(body) }),
  deleteExtra: (planId: string, id: number) => request<void>(`/api/plans/${planId}/extras/${id}`, { method: "DELETE" }),
};

import { tStatic } from "../i18n";
export type Labeled = { id: string; label: string };

export type Store = { code: string; country: string; name: string; kind: string; priceIndex: number; note: string; sort: number };

export type Country = {
  code: string;
  label?: string;
  currency: string;
  symbol: string;
  decimals: number;
  locale: string;
  hasRegions: boolean;
  presets?: { id: string; label: string; perDay: number }[];
  default?: number;
};

export type Region = { code: string; name: string; kind: "rf" | "district" | "region" | "city"; parent: string };

export type FormulaBrand = { id: string; name: string; packG: number; factor: number; note: string };

export type Meta = {
  lang: string;
  countries: Country[];
  stores: Store[];
  regions: Region[];
  prices: { source: string; period: string; weeklyDate: string } | null;
  localPrices: Record<string, { source: string; period: string }>;
  allergens: Labeled[];
  equipment: Labeled[];
  slots: Labeled[];
  goals: { id: string; label: string; kcal: number }[];
  feeding: Labeled[];
  budgetPresets: { id: string; label: string; perDay: number }[];
  formulaBrands: FormulaBrand[];
  weaningFoods?: WeaningFood[];
  excludePresets: { id: string; label: string; kind: "ingredient" | "tag" }[];
  ingredients: Labeled[];
  recipes: number;
  geoCountry: string; // страна по IP посетителя, если среди поддерживаемых; иначе ""
  geoRegion?: string; // регион или город Росстата по IP (только для стран с регионами)
  ai?: boolean; // помощник (нейросеть) для своих рецептов включён на сервере
  photos?: boolean; // загрузка фото включена (есть S3)
  bots?: Partial<Record<"telegram" | "max", string>>; // имена ботов: по ним кабинет показывает привязку мессенджера
  sample?: { country: string; dishes: number; items: number; cost: number; store: string }; // неделя на двоих при стартовых ответах
};

export type Child = {
  name?: string;
  ageMonths: number;
  feeding: "shared" | "separate" | "jars" | "milk" | string;
  sharesMeals: boolean;
  formula: boolean;
  formulaBrand: string;
  formulaMl: number;
  formulaFeeds?: string[]; // когда даёт смесь: morning | day | bedtime | night; пусто — во все кормления
  meals?: Record<string, string>; // режим «комбинирую»: приём → home | jars | shared
  allergens?: string[]; // аллергии самого ребёнка (коды как у семьи)
  ageAt?: string; // месяц (YYYY-MM), когда указан возраст: дальше он растёт сам
  avoid?: string[]; // продукты, на которые была реакция: в прикорм и меню ребёнка не ставятся
  away?: string; // по будням: "" дома, kindergarten — в саду, school — в школе
  introduced?: string[]; // прикорм: продукты, которые малыш уже ест
};

// Неделя прикорма ребёнка 4–11 мес (backend/internal/planner/weaning.go).
export type WeaningItem = { food: string; recipe?: string; name: string; group: string; grams: number; unit: "g" | "ml" | "pcs" | string; new: boolean };
export type WeaningFeed = { time: string; milk: boolean; milkKind?: "formula" | "breast"; milkMl?: number; items: WeaningItem[] };
export type Weaning = {
  month: number;
  days: { index: number; label: string; feeds: WeaningFeed[] }[];
  new?: WeaningItem;
  newName?: string; // название продукта недели без «Пюре:»
  ramp?: number[];
  next: string[];
  introduced: string[];
  texture: "puree" | "mashed" | string;
  milkMl: number;
};
export type WeaningFood = { id: string; group: string; from: number; name: string; img?: string };

export type Params = {
  lang?: string;
  country: string;
  store: string;
  region: string;
  adults: number;
  members: Member[];
  kids: Child[];
  goal: string;
  kcalTarget: number;
  budgetMode: "perPersonDay" | "week";
  budgetValue: number;
  compact?: boolean; // «меньше разных продуктов»
  allergens: string[];
  exclude: string[];
  excludeTags: string[];
  equipment: string[];
  slots: string[];
  startDate?: string;
  prep?: "" | "one" | "two"; // заготовки: каждый день | раз в неделю | два раза
  wants: string[];
  collection?: string; // неделя из коллекции (id)
  have: string[]; // что уже есть дома: планировщик использует первым, список покупок вычитает упаковку
};

// Едок семьи: цель, аппетит и приёмы дома (пусто — все).
export type Member = { name: string; goal: string; appetite: "small" | "normal" | "big"; slots: string[]; mom?: string };
// беременность по триместрам или кормление грудью: прибавка к ккал мамы и блюда, которых при беременности избегают
export const MOM_STATES = ["", "pregnant1", "pregnant2", "pregnant3", "nursing"] as const;
export type MemberView = { name: string; goal: string; goalLabel: string; kcal: number; factor: number; slots: string[] };
export const APPETITES = ["small", "normal", "big"] as const;

export type Dish = {
  kidNote?: string; // как подать малышу, который ест с общего стола: «виноград — на четвертинки»
  servings?: number; // стол: на сколько человек готовят это блюдо
  slot: string;
  recipeId: string;
  title: string;
  timeMin: number;
  leftover: boolean;
  batch: boolean;
  kcal: number;
  protein: number;
  fat: number;
  carb: number;
  cost: number;
  own?: boolean; // рецепт пользователя
  course?: string; // событие: курс вместо приёма пищи
  why: string;
  whyCode?: unknown;
  side?: { recipeId: string; title: string; timeMin: number; kcal: number; protein: number; fat: number; carb: number; cost: number }; // гарнир; итоги блюда — суммой с ним
  prep?: { session: number; mode: "fridge" | "freezer" | "fresh" }; // режим заготовок
};

export type PrepDay = { index: number; date: string; label: string; items: { recipeId: string; title: string; timeMin: number; mode: "fridge" | "freezer"; forDays: number[]; portions: number; side?: string; sideId?: string }[]; totalMin: number };

export type Day = {
  index: number;
  date: string;
  label: string;
  dishes: Dish[];
  kcal: number;
  protein: number;
  fat: number;
  carb: number;
  cost: number;
  cookMin: number;
  perMember?: number[];
  skipped?: boolean; // «не дома»: не считается в покупках и итогах
};

export type ShopItem = {
  ingredientId: string;
  name: string;
  category: string;
  unit: string;
  needed: number;
  buy: number;
  packs: number;
  pack: number;
  loose: boolean;
  pantry: boolean;
  cost: number;
  rosstat: boolean;
  home?: number; // сколько уже есть дома (вычтено из buy)
  image?: string; // картинка продукта для подсказки
  atHome?: boolean; // дома хватает на всю неделю
  usedIn: string[];
};

export type ShopGroup = { category: string; label: string; items: ShopItem[]; cost: number };

export type Plan = {
  id: string;
  basket?: { guests: number; dishes: number }; // стол, набранный вручную, вместо недели
  params: Params;
  prepDays?: PrepDay[];
  lang: string;
  country: Country;
  store: Store;
  portions: number;
  days: Day[];
  shopping: ShopGroup[];
  kidsMenus: { child: number; ageLabel: string; factor: number; note: string; norm?: string; weaning?: Weaning; days: { index: number; label: string; dishes: { slot: string; recipeId: string; title: string; timeMin: number; kcal: number; cost: number; kind?: "jars" | "shared" | "none" | "bedtime" | "away.kindergarten" | "away.school"; ref?: string; note?: string }[] }[] }[];
  totals: {
    cost: number;
    pantryCost: number;
    babyCost: number;
    kidsMenuCost: number;
    usedCost: number;
    homeSaved: number;
    items: number;
    kcalPerDay: number;
    proteinPerDay: number;
    fatPerDay: number;
    carbPerDay: number;
    cookMin: number;
  };
  budget: { mode: string; value: number; perDay: number; targetWeek: number };
  goal: { level: string; label: string; kcalTarget: number };
  members?: MemberView[];
  slotPortions?: Record<string, number>;
  family?: string[];
  bots?: Partial<Record<"telegram" | "max", string>>; // ссылки на ботов: по ним неделя подключается и список приходит в чат
  occasion?: { id: string; title: string; guests: number };
  // store/storeDate/storeCoverage — цены из каталога самой сети (ВкусВилл) и доля корзины по ним
  priceSource: { name: string; period: string; weeklyDate: string; region: string; regionCode: string; coverage: number; store?: string; storeDate?: string; storeCoverage?: number };
  seed: number;
  swaps: number;
  generatedAt: string;
  warnings: string[];
  notes: string[];
};

export type Recipe = {
  id: string;
  title: string;
  description: string;
  image: string;
  slot: string;
  slotLabel: string;
  timeMin: number;
  equipment: string[];
  tags: string[];
  batch: boolean;
  steps: string[];
  ingredients: { ingredientId: string; name: string; amount: number; unit: string; pantry: boolean; image?: string }[];
  kcal: number;
  protein: number;
  fat: number;
  carb: number;
  cost: number; // цена порции в валюте страны запроса
  own?: boolean;
  public?: boolean;
  author?: string;
  stats?: RecipeStats;
};

// Свой рецепт: продукты из общей базы с количеством на порцию.
export type OwnIngredient = { ingredientId: string; amount: number };
export type TranslationSummary = { done: number; total: number; errors: number; running?: string; model?: string };
export type TranslationStatus = { source: string; enabled: boolean; items: { lang: string; status: "queued" | "running" | "done" | "error"; model?: string; error?: string; updatedAt: string }[]; providers: string[] };

export type OwnRecipe = {
  id: string;
  public?: boolean;
  translations?: TranslationSummary;
  views?: number;
  likes?: number;
  comments?: number;
  favorites?: number;
  title: string;
  description: string;
  slot: string;
  slotLabel?: string;
  timeMin: number;
  equipment: string[];
  tags: string[];
  steps: string[];
  ingredients: OwnIngredient[];
  image?: string; // фото из нашего хранилища
  status?: "private" | "checking" | "review" | "improve" | "approved" | "rejected";
  note?: string; // причина от нейросети или модератора
  suggestion?: { title: string; description: string; steps: string[] }; // подробная версия от нейросети (status = improve)
  kcal?: number;
  cost?: number;
};
export type OwnRecipeInput = Omit<OwnRecipe, "id" | "slotLabel" | "kcal" | "cost" | "public">;
export type IngredientRef = { id: string; label: string; unit: string; pantry: boolean; category: string };
export const OWN_TAGS = ["pp", "protein", "soup", "salad", "vegetarian", "sweet", "spicy", "hearty"];

export const slotLabel = (_lang: string, slot: string) => tStatic(`slot.${slot}`);

export type User = { id: string; email: string; name: string; nick: string; avatar?: string; defaults: Partial<Params> | Record<string, never> };
export type RecipeStats = { likes: number; comments: number; liked: boolean; favorite: boolean };
export type Comment = { id: number; nick: string; name: string; body: string; html: string; image?: string; avatar?: string; createdAt: string; mine: boolean };
export type FamilyAccount = { userId: string; name: string; nick: string; owner: boolean; you: boolean };
export type Family = { id: string; name: string; adults: Member[]; kids: Child[]; accounts: FamilyAccount[]; owner: boolean; inviteToken?: string };
export type Favorite = { id: string; title: string; slot: string; own: boolean; image?: string };
export type PlanSummary = { id: string; title: string; startDate: string; store: string; cost: number; country?: Country; portions: number; createdAt: string; checked: number; items: number; shared?: boolean; occasion?: { id: string; title: string; guests: number }; date?: string };
export type NotifySettings = { shopDay: number; shopHour: number; today: boolean; todayHour: number; prep: boolean; prepHour: number; prepDay: boolean; week: boolean; digest: boolean; noAsk?: boolean; tz: number }; // shopHour -1 — напоминание о магазине выключено
export type AdminOverview = {
  counters: { users: number; usersWeek: number; activeWeek: number; plans: number; plansWeek: number; plansOwned: number; ownRecipes: number; households: number; pushUsers: number; comments: number; feedback: number; purchasesWeek: number; errorsWeek: number; secondWeekBase: number; secondWeek: number };
  daily: { day: string; users: number; plans: number; visitors: number; quizStarts: number; errors: number }[];
  events: { name: string; count: number; sessions: number }[];
  top: { stores: { key: string; count: number }[] | null; recipes: { key: string; count: number }[] | null };
};
export type AdminUser = { id: string; email: string; name: string; createdAt: string; plans: number; lastSeen?: string; role: string };
export type AdminRecipe = { id: string; title: string; description: string; slot: string; timeMin: number; batch: boolean; equipment: string[]; tags: string[]; steps: string[]; ingredients: OwnIngredient[]; image: string; hidden?: boolean };
export type CatalogRecipeInput = Omit<AdminRecipe, "id"> & { id: string };
export type ModerationItem = AdminRecipe & { author?: string; status?: string; note?: string; submittedAt?: string };
export type AdminError = { at: string; sid: string; message: string; url: string; stack: string; ua: string };
export type AdminLog = { time: string; level: string; logger?: string; msg: string; fields?: Record<string, unknown> };
export type Collection = { id: string; name: string; recipes: string[]; createdAt: string; public?: boolean; curated?: boolean; slug?: string; description?: string; cover?: string; coverAuto?: string; author?: string; names?: Record<string, string>; descriptions?: Record<string, string> };
export type OccasionView = { id: string; icon: string; title: string; lead: string; guests: number; kind: "menu" | "week"; season: boolean; preset?: { excludeTags: string[]; allergens: string[]; goal: string }; courses: string[]; countries?: string[] };
export type SubOption = { id: string; name: string; amount: number; unit: string; note: string; kcalDelta: number; costDelta: number; symbol?: string };
export type SubRow = { ingredientId: string; options: SubOption[] };
export type BudgetReport = { weeks: { start: string; planned: number; bought: number }[]; months: { month: string; bought: number }[]; deltaPct?: number | null; currency: string };
export type Purchase = { id: number; planId: string | null; itemId: string; name: string; qty: string; cost: number; boughtAt: string };
export type Extra = { id: number; name: string; qty: string; due: string | null; note: string };

// Партнёрский магазин: шаблон ссылки с {q}; affiliate — показываем «Реклама» и erid.
export type Partner = { code: string; country: string; kind: "goods" | "grocery"; name: string; url: string; affiliate: boolean; erid: string; active: boolean; priority: number };
export type PartnerView = { goods: Partner[]; grocery: Partner[]; queries: Record<string, string> };
export type Offer = {
  id: string; country: string; regions: string[]; place: "cart" | "recipe" | "plan"; match: string[];
  title: string; body: string; cta: string; url: string; image: string; promo: string;
  affiliate: boolean; erid: string; startsAt: string | null; endsAt: string | null; active: boolean; priority: number;
};
export type ApiKey = { id: string; name: string; prefix: string; createdAt: string; lastUsedAt?: string; key?: string };

// Корзина сети ссылкой: что положили и что осталось искать по ссылкам на поиск.
export type StoreCart = { link: string; added: number; matched: number; wanted: number; left: string[] };

// ФТ-01: профиль и рассчитанная норма (см. backend/internal/nutrition)
export interface UserProfile {
  sex: "m" | "f";
  birthDate: string;
  heightCm: number;
  weightKg: number;
  bodyFatPct?: number;
  activity: string;
  goal: string;
  pace: string;
  updatedAt?: string;
}

export interface ProfileTargets {
  method: "mifflin" | "cunningham";
  age: number;
  bmi: number;
  leanMassKg?: number;
  rmr: number;
  tdee: number;
  kcal: number;
  proteinG: number;
  fatG: number;
  carbsG: number;
  shiftPct: number;
  weeklyKg: number;
  floorApplied: boolean;
  notes?: string[];
}

// ФТ-05: журнал веса (см. backend/internal/journal)
export interface WeightPoint {
  date: string;
  weight?: number;
  trend?: number;
}

export interface WeightCoverage {
  days: number;
  weighIns: number;
  freshDay: number;
}

export interface WeightView {
  from: string;
  to: string;
  points: WeightPoint[];
  last?: { date: string; kg: number; note?: string };
  ratePerWeek?: number;
  coverage: WeightCoverage;
  ready: boolean;
  blockers?: string[];
}

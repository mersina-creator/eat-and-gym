-- Профиль для расчёта нормы калорий и БЖУ (ФТ-01). Одна строка на аккаунт.
-- body_fat_pct — из выгрузки состава тела; NULL — неизвестен, тогда расчёт по Миффлину — Сан Жеору.
CREATE TABLE IF NOT EXISTS user_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    sex          CHAR(1)  NOT NULL CHECK (sex IN ('m', 'f')),
    birth_date   DATE     NOT NULL,
    height_cm    NUMERIC(5,1) NOT NULL,
    weight_kg    NUMERIC(5,1) NOT NULL,
    body_fat_pct NUMERIC(4,1),
    activity     TEXT     NOT NULL,
    goal         TEXT     NOT NULL,
    pace         TEXT     NOT NULL DEFAULT 'normal',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

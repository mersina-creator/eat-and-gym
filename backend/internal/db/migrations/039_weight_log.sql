-- ФТ-05, шаг 1: журнал веса. Одно взвешивание в день, ключ (user_id, date):
-- повторный ввод за ту же дату перезаписывает значение, а не плодит строки.
-- date — календарная дата с устройства, а не now(): взвешивание в 23:50 по Москве
-- не должно уехать на следующие сутки по UTC.
CREATE TABLE IF NOT EXISTS weight_log (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    weight_kg  NUMERIC(5,1) NOT NULL CHECK (weight_kg >= 30 AND weight_kg <= 400),
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, date)
);

CREATE INDEX IF NOT EXISTS weight_log_user_date ON weight_log (user_id, date DESC);

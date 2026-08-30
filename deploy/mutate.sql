-- Изменяет :pct процентов записей каталога.
-- Выбор детерминирован (по article), чтобы замеры воспроизводились.
UPDATE parts
SET material = material || ' (ред.)'
WHERE id IN (
    SELECT id FROM parts
    WHERE deleted_at IS NULL
    ORDER BY article
    LIMIT (SELECT count(*) * :pct / 100 FROM parts WHERE deleted_at IS NULL)
);

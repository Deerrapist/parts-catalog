-- Монотонный счётчик версий, общий на всю таблицу parts.
CREATE SEQUENCE global_version_seq;

CREATE TABLE categories (
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE parts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID REFERENCES categories(id),
    name        TEXT NOT NULL,
    article     TEXT NOT NULL UNIQUE,
    size        TEXT,
    material    TEXT,
    weight_g    INTEGER,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    version     BIGINT NOT NULL DEFAULT nextval('global_version_seq')
);

-- Индексы под будущую дельту: клиент спрашивает "всё, что новее версии N".
CREATE INDEX idx_parts_version    ON parts (version);
CREATE INDEX idx_parts_updated_at ON parts (updated_at);
CREATE INDEX idx_parts_category   ON parts (category_id);

-- Ключевое: DEFAULT работает только на INSERT.
-- Без этого триггера правки и soft delete до клиента не доедут.
CREATE FUNCTION bump_version() RETURNS trigger AS $$
BEGIN
    NEW.version    := nextval('global_version_seq');
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER parts_bump_version
    BEFORE UPDATE ON parts
    FOR EACH ROW
    WHEN (OLD.* IS DISTINCT FROM NEW.*)
    EXECUTE FUNCTION bump_version();

-- Тестовые данные (немного, для проверки; 500 записей будут на этапе замеров)
INSERT INTO categories (name) VALUES ('Крепёж'), ('Подшипники'), ('Уплотнения');

INSERT INTO parts (category_id, name, article, size, material, weight_g, description)
SELECT c.id, 'Болт М10х40', 'BLT-M10-40', 'М10х40', 'Сталь 40Х', 32, 'Болт с шестигранной головкой'
FROM categories c WHERE c.name = 'Крепёж';

INSERT INTO parts (category_id, name, article, size, material, weight_g, description)
SELECT c.id, 'Подшипник 6204', 'BRG-6204', '20x47x14', 'ШХ15', 106, 'Радиальный шариковый однорядный'
FROM categories c WHERE c.name = 'Подшипники';
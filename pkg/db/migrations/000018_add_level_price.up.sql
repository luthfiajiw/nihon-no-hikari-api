ALTER TABLE public.levels
    ADD COLUMN price integer NOT NULL DEFAULT 0
    CONSTRAINT levels_price_check CHECK (price >= 0 AND price <= 100000000);

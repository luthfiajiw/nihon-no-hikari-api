ALTER TABLE public.users
ADD COLUMN role varchar(10) NOT NULL DEFAULT 'User'
CONSTRAINT users_role_check CHECK (role IN ('User', 'Admin'));

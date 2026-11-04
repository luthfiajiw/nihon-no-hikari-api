CREATE TABLE public.user_identities (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  provider      varchar(20) NOT NULL,
  provider_sub  varchar(255) NOT NULL,
  email         varchar(255) NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT user_identities_provider_sub_key UNIQUE (provider, provider_sub),
  CONSTRAINT user_identities_user_provider_key UNIQUE (user_id, provider)
);

CREATE INDEX user_identities_user_id_idx ON public.user_identities(user_id);

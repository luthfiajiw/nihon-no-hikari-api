CREATE TABLE public.email_verifications (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  purpose           varchar(50) NOT NULL,
  code_hash         varchar(255) NOT NULL,
  attempts          int NOT NULL DEFAULT 0,
  expires_at        timestamptz NOT NULL,
  verified_at       timestamptz,
  grant_hash        varchar(255),
  grant_expires_at  timestamptz,
  consumed_at       timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_verifications_user_purpose_idx ON public.email_verifications (user_id, purpose, created_at DESC);
CREATE INDEX email_verifications_grant_hash_idx ON public.email_verifications (grant_hash) WHERE grant_hash IS NOT NULL;

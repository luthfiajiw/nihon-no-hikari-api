ALTER TABLE user_sessions DROP CONSTRAINT IF EXISTS user_sessions_id_users_fk;
DROP INDEX IF EXISTS fk_user_sessions_users_idx;
DROP TABLE IF EXISTS "user_sessions";
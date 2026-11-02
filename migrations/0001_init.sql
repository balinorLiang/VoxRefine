CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL,
  password_hash text NOT NULL,
  display_name text NOT NULL,
  role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'moderator', 'admin')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_uidx ON users (lower(email));

CREATE TABLE IF NOT EXISTS sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions (expires_at);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions (user_id);

CREATE TABLE IF NOT EXISTS terms (
  id text PRIMARY KEY,
  word text NOT NULL,
  pattern text NOT NULL DEFAULT '',
  context_after text[] NOT NULL DEFAULT '{}',
  type text NOT NULL,
  risk text NOT NULL CHECK (risk IN ('high', 'medium')),
  default_pinyin text NOT NULL,
  note text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS terms_word_trgm_idx ON terms USING gin (word gin_trgm_ops);
CREATE INDEX IF NOT EXISTS terms_note_trgm_idx ON terms USING gin (note gin_trgm_ops);
CREATE INDEX IF NOT EXISTS terms_type_idx ON terms (type);
CREATE INDEX IF NOT EXISTS terms_risk_idx ON terms (risk);

CREATE TABLE IF NOT EXISTS readings (
  id bigserial PRIMARY KEY,
  term_id text NOT NULL REFERENCES terms(id) ON DELETE CASCADE,
  pinyin text NOT NULL,
  note text NOT NULL DEFAULT '',
  is_wrong boolean NOT NULL DEFAULT false,
  sort_order integer NOT NULL DEFAULT 0,
  UNIQUE (term_id, pinyin)
);

CREATE INDEX IF NOT EXISTS readings_term_idx ON readings (term_id);

CREATE TABLE IF NOT EXISTS cases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  term_id text REFERENCES terms(id) ON DELETE SET NULL,
  wrong_text text NOT NULL,
  correct_pinyin text NOT NULL,
  note text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  moderation_note text NOT NULL DEFAULT '',
  moderated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  moderated_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS cases_status_created_idx ON cases (status, created_at DESC);
CREATE INDEX IF NOT EXISTS cases_term_idx ON cases (term_id);
CREATE INDEX IF NOT EXISTS cases_user_idx ON cases (user_id);

CREATE TABLE IF NOT EXISTS case_votes (
  case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (case_id, user_id)
);

CREATE INDEX IF NOT EXISTS case_votes_user_idx ON case_votes (user_id);
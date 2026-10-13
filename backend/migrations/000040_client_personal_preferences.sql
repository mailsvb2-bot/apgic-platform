-- Personal presentation preferences only; booking, orders, consultation and
-- consent are read from their existing canonical owners.
CREATE TABLE client_personal_preferences (
  identity_id uuid PRIMARY KEY REFERENCES identities(id) ON DELETE RESTRICT,
  display_name text NOT NULL DEFAULT '' CHECK (length(display_name) <= 320),
  locale text NOT NULL DEFAULT 'ru' CHECK (locale IN ('ru','en')),
  time_zone text NOT NULL DEFAULT 'Europe/Moscow' CHECK (length(time_zone) BETWEEN 1 AND 64),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX client_personal_preferences_updated_idx ON client_personal_preferences(updated_at);

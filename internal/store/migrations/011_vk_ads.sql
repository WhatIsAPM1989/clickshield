ALTER TABLE sessions ADD COLUMN rb_clickid text NOT NULL DEFAULT '';
ALTER TABLE integrations DROP CONSTRAINT integrations_provider_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_provider_check
    CHECK (provider IN ('YANDEX_METRICA', 'GOOGLE_ANALYTICS', 'VK_ADS'));

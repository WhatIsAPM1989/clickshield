-- Verifiable crawler user agents from addresses outside their operators'
-- published ranges are now labeled as spoofed, so re-label those sessions.
UPDATE sessions SET bot_name=NULL WHERE bot_name IN ('Googlebot', 'Googlebot-Image', 'Googlebot-Video', 'Googlebot-News', 'GoogleOther', 'Google StoreBot', 'Google AdsBot', 'Google AdSense', 'Bingbot', 'Applebot', 'DuckDuckBot', 'GPTBot', 'ChatGPT-User', 'OAI-SearchBot', 'PerplexityBot');

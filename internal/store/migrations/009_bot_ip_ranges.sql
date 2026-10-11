-- Bots are now also identified by published crawler IP ranges (for example
-- Bing rendering pages with HeadlessChrome), so every session is re-labeled.
UPDATE sessions SET bot_name=NULL WHERE bot_name IS NOT NULL;

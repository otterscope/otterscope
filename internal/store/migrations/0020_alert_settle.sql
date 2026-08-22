-- Flap suppression for alerts (#108). Without it a value hovering at the
-- threshold produced a firing/resolved pair every evaluation interval —
-- two Slack messages a minute at the default interval, which is how an
-- alert channel gets muted and the real alert gets missed.
--
-- settle_secs: how long the condition must hold its new state before we
-- notify. 0 keeps the previous behaviour (notify on the first evaluation
-- that flips), so existing alerts are unchanged by this migration.
-- pending_since_ns: when the condition first flipped away from the state we
-- last notified about; 0 means nothing is pending.
ALTER TABLE alerts ADD COLUMN settle_secs INTEGER NOT NULL DEFAULT 0;
ALTER TABLE alerts ADD COLUMN pending_since_ns INTEGER NOT NULL DEFAULT 0;

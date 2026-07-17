-- Named, saved filter/search combinations ("my dashboards").
CREATE TABLE saved_views (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    params     TEXT NOT NULL DEFAULT '{}',
    created_ns INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS images (
    id INTEGER PRIMARY KEY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);

CREATE TABLE IF NOT EXISTS sboms (
    id INTEGER PRIMARY KEY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);

CREATE TABLE IF NOT EXISTS vulnerabilityreports (
    id INTEGER PRIMARY KEY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);
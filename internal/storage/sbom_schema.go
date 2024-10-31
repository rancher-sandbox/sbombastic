package storage

const CreateSBOMTableSQL = `
CREATE TABLE IF NOT EXISTS sboms (
    id INTEGER PRIMARY KEY,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    object TEXT NOT NULL,
    UNIQUE(name, namespace)
);
`

// sbomSchema is the schema for the sbom table
// Note: the struct fields must be exported in order to work.
type sbomSchema struct {
	ID        int    `db:"id"`
	Name      string `db:"name"`
	Namespace string `db:"namespace"`
	Object    []byte `db:"object"`
}

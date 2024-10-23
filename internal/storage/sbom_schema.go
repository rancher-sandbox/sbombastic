package storage

// sbomSchema is the schema for the sbom table
// Note: the struct fields must be exported in order to work.
type sbomSchema struct {
	Key    string `db:"key"`
	Object []byte `db:"object"`
}

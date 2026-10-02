// The schemas hive publishes. Each entry names a CUE package under this
// module and the definition that is the contract; `mise run generate:schemas`
// exports that definition as JSON Schema, copies it into the docs site, and
// writes a Go type where an entry asks for one.
package schema

#Entry: {
	id!:          =~"^[a-z]+\\.[a-z_]+$"
	title!:       string
	description!: string
	// CUE package directory under this module.
	dir!: string
	// The definition exported as <dir>/<id>.schema.json.
	definition!: =~"^#"
	// Where `cue exp gengotypes` writes the Go types, relative to the repo root.
	go_out?: string
	// The page on hivedesktop.com that documents the format.
	doc!: =~"^/"
}

catalog: [...#Entry] & [{
	id:          "hc.tree"
	title:       "hc bulk-create tree"
	description: "The JSON `hive hc create` reads from stdin or --file: an epic and its children, with local refs and blockers."
	dir:         "hc"
	definition:  "#CreateInput"
	go_out:      "internal/core/hc/create_input.gen.go"
	doc:         "/cli/getting-started/task-tracking/"
}]

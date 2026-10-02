package hc

import (
	"list"
	"strings"
)

// CreateInput is one node of a bulk-create tree: the root epic or any item
// beneath it. The tree is walked breadth first and ids are generated on
// creation, so a node names nothing by id.
#CreateInput: {
	// Local label other nodes name in blockers; not stored.
	ref?: =~"^[a-z0-9][a-z0-9_-]*$"
	title!: string & strings.MinRunes(1) & strings.MaxRunes(200)
	desc?:  string
	type!:  "epic" | "task" @go(,type=ItemType)
	// Child items, created in the same call.
	children?: [...#CreateInput] @go(,type=[]CreateInput)
	// Refs of nodes in this tree that must complete before this one.
	blockers?: [...string]
}

// Tree is the root as `cue vet -c -d '#Tree' file.json` checks it: an epic
// whose blockers all name a ref in the tree, with unique refs and at most five
// levels. It is not the exported schema and generates no Go type; the same
// rules run in Go at runtime through hive.ValidateCreateInput.
#Tree: #CreateInput & {
	type: "epic"
	ref?: string
	children?: [...#CreateInput]

	_l1: [if children != _|_ for c in children {c}]
	_l2: [for n in _l1 if n.children != _|_ for c in n.children {c}]
	_l3: [for n in _l2 if n.children != _|_ for c in n.children {c}]
	_l4: [for n in _l3 if n.children != _|_ for c in n.children {c}]
	_l5: [for n in _l4 if n.children != _|_ for c in n.children {c}]
	_all: list.Concat([_l1, _l2, _l3, _l4])
	_refs: list.Concat([[if ref != _|_ {ref}], [for n in _all if n.ref != _|_ {n.ref}]])

	_depth_ok:    (len(_l5) == 0) & true
	_refs_unique: list.UniqueItems(_refs) & true
	_blockers_resolve: {
		for n in _all if n.blockers != _|_ for b in n.blockers {
			(b): list.Contains(_refs, b) & true
		}
	}
} @go(-)

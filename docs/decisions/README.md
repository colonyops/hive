# Decisions

Architecture and infrastructure decisions, one file per decision. The directory
listing is the index: the filename's date orders it, the slug names it.

Prose cites the slug alone: `(ADR terminal-transport)`. There is no number to
allocate, so two branches can add an ADR without colliding. Add one with
`mise run adr:new -- "Title of the decision"`; `mise run check:adr` keeps ids
and citations consistent. Superseded ADRs are marked, not deleted.

// Package kg implements a bitemporal knowledge graph backed by SQLite.
//
// # Data Model
//
// The KG stores SPO (Subject-Predicate-Object) triples about entities extracted
// from conversation memories. Each triple carries two time dimensions:
//
//   - Valid time (valid_from/valid_until): when the fact is true in the real world
//   - Transaction time (txn_from/txn_until): when the row exists in the database
//
// Functional predicates enforce that only one triple per (subject, predicate) may
// be currently valid — inserting a new value automatically invalidates the old one.
//
// Entity aliases enable case and accent-insensitive matching: "Maria", "maria",
// and "María" all resolve to the same entity_id.
//
// # ADR References
//
//   - ADR-003: KG shares the same SQLite file as the memory store (no separate DB)
//   - ADR-009: Parameterized SQL required for all user-facing queries
//
// # Wing Inheritance
//
// Triples inherit the wing of their source memory. A triple with wing IS NULL
// is neutral — never filtered, never penalized, never downgraded.
package kg

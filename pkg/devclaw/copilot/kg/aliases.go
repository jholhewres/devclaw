package kg

import (
	"context"
	"strings"
)

func normalizeAlias(name string) string {
	return strings.TrimSpace(stripAccents(strings.ToLower(name)))
}

func stripAccents(s string) string {
	var replacements = map[rune]rune{
		'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
		'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
		'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
		'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o',
		'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
		'ç': 'c',
		'ñ': 'n',
		'ý': 'y', 'ÿ': 'y',
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if rep, ok := replacements[r]; ok {
			b.WriteRune(rep)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (k *KG) ResolveAlias(ctx context.Context, name string) (int64, error) {
	normalized := normalizeAlias(name)
	var entityID int64
	err := k.db.QueryRowContext(ctx,
		"SELECT entity_id FROM kg_entity_aliases WHERE alias_name = ?",
		normalized,
	).Scan(&entityID)
	return entityID, err
}

func (k *KG) RegisterAlias(ctx context.Context, entityID int64, alias string) error {
	normalized := normalizeAlias(alias)
	_, err := k.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO kg_entity_aliases (entity_id, alias_name) VALUES (?, ?)",
		entityID, normalized,
	)
	return err
}

func (k *KG) AutoRegisterAlias(ctx context.Context, entityID int64, canonicalName string) error {
	if err := k.RegisterAlias(ctx, entityID, canonicalName); err != nil {
		return err
	}
	normalized := normalizeAlias(canonicalName)
	lowered := strings.TrimSpace(strings.ToLower(canonicalName))
	if normalized != lowered {
		if err := k.RegisterAlias(ctx, entityID, normalized); err != nil {
			return err
		}
	}
	return nil
}

package kg

// PII_PREDICATE_BLACKLIST lists predicates that are ALWAYS dropped
// before insertion, regardless of source. This is a data-type classification,
// not a language keyword list.
var PII_PREDICATE_BLACKLIST = map[string]bool{
	"ssn":          true,
	"credit_card":  true,
	"password":     true,
	"address":      true,
	"cpf":          true,
	"rg":           true,
	"phone":        true,
	"email":        true,
	"bank_account": true,
}

// IsPIIPredicate returns true if the predicate is in the blacklist.
func IsPIIPredicate(predicate string) bool {
	return PII_PREDICATE_BLACKLIST[predicate]
}

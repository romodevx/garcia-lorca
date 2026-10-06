package domain

import "strings"

// stateAliases are the long official names and the everyday ones a previous
// system wrote, keyed by their folded form, mapped to the canonical key of
// mexicanStates. "MEXICO" alone is the state, never the country: a home address
// already is in the country.
var stateAliases = map[string]string{
	"michoacan de ocampo":             "Michoacán",
	"cdmx":                            "Ciudad de México",
	"df":                              "Ciudad de México",
	"distrito federal":                "Ciudad de México",
	"mexico":                          "Estado de México",
	"edo mex":                         "Estado de México",
	"edomex":                          "Estado de México",
	"edo de mexico":                   "Estado de México",
	"estado de mexico":                "Estado de México",
	"veracruz de ignacio de la llave": "Veracruz",
	"coahuila de zaragoza":            "Coahuila",
	"queretaro de arteaga":            "Querétaro",
}

// statesByFold indexes the 32 canonical names and every alias by folded form.
var statesByFold = func() map[string]string {
	out := make(map[string]string, len(mexicanStates)+len(stateAliases))
	for name := range mexicanStates {
		out[foldStateName(name)] = name
	}
	for alias, name := range stateAliases {
		out[alias] = name
	}
	return out
}()

// MatchState resolves a state as a person reads it — without case, accents,
// dots or extra spaces, and with the usual aliases — to its canonical name.
func MatchState(raw string) (string, bool) {
	name, ok := statesByFold[foldStateName(raw)]
	return name, ok
}

// unaccent folds the Spanish accented vowels (state names carry no other
// diacritic) and turns dots into spaces ("Edo. Mex.").
var unaccent = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", ".", " ",
)

// foldStateName lowercases, strips accents and dots, and collapses spaces.
func foldStateName(s string) string {
	return strings.Join(strings.Fields(unaccent.Replace(strings.ToLower(s))), " ")
}

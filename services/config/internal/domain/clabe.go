package domain

import (
	glerrors "github.com/garcia-lorca/pkg/errors"
)

const (
	clabeDigits     = 18
	clabeBankDigits = 3
)

// clabeWeights are Banxico's, repeated over the first seventeen digits.
var clabeWeights = [clabeDigits - 1]int{3, 7, 1, 3, 7, 1, 3, 7, 1, 3, 7, 1, 3, 7, 1, 3, 7}

// clabeBanks names the bank behind a CLABE's first three digits (the ABM code),
// for the banks a family is likely to see. An unknown code names nobody: the
// CLABE is still valid, the app just does not print a bank.
var clabeBanks = map[string]string{
	"002": "Banamex",
	"012": "BBVA México",
	"014": "Santander",
	"021": "HSBC",
	"030": "BanBajío",
	"036": "Inbursa",
	"044": "Scotiabank",
	"058": "Banregio",
	"062": "Afirme",
	"072": "Banorte",
	"127": "Banco Azteca",
	"137": "BanCoppel",
	"638": "Nu México",
	"646": "STP",
}

// normaliseCLABE strips the spaces a person types between groups and checks
// the eighteen digits and Banxico's check digit; empty means none. One
// mistyped digit would send a family's transfer to somebody else.
func normaliseCLABE(raw string) (string, error) {
	clabe := withoutSeparators(raw)
	if clabe == "" {
		return "", nil
	}
	if len(clabe) != clabeDigits || digitsOf(clabe) != clabe || clabeCheckDigit(clabe) != int(clabe[clabeDigits-1]-'0') {
		return "", glerrors.New(glerrors.KindInvalid, "the CLABE must be eighteen digits with a valid check digit")
	}
	return clabe, nil
}

// clabeCheckDigit is Banxico's: each digit times its weight, modulo 10, summed;
// the check digit is what takes that sum to the next multiple of ten.
func clabeCheckDigit(clabe string) int {
	sum := 0
	for i, w := range clabeWeights {
		sum += int(clabe[i]-'0') * w % 10
	}
	return (10 - sum%10) % 10
}

// BankOfCLABE names the bank of a valid CLABE, or "" when the code is unknown.
func BankOfCLABE(clabe string) string {
	if len(clabe) != clabeDigits {
		return ""
	}
	return clabeBanks[clabe[:clabeBankDigits]]
}

// withoutSeparators drops spaces and dashes only, so letters survive to be refused.
func withoutSeparators(raw string) string {
	out := make([]rune, 0, len(raw))
	for _, r := range raw {
		if r != ' ' && r != '\t' && r != '-' {
			out = append(out, r)
		}
	}
	return string(out)
}

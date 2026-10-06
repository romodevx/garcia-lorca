package legacyimport

import (
	"strconv"
	"strings"
	"time"
)

// Skip reasons: rows the import leaves out, as stable codes the screen words.
const (
	SkipWithdrawn      = "status_withdrawn" // "Baja" in the previous system (O3)
	SkipStatusUnknown  = "status_unknown"
	SkipGradeInvalid   = "grade_invalid"
	SkipGroupInvalid   = "group_invalid"
	SkipLevelMissing   = "level_missing"
	SkipNameUnreadable = "name_unreadable"
)

// Birth date warnings.
const (
	WarnBirthDateFromCURP = "birth_date_from_curp"
	WarnBirthDateInvalid  = "birth_date_invalid"
)

const (
	statusActive    = "activo"
	statusWithdrawn = "baja"
	maxGroupLen     = 10
	curpLen         = 18
)

// Candidate is a row read and decided as far as the spreadsheet alone allows.
// Whether the CURP is valid, already known, or the profile is well formed is
// for students to judge; whether the level exists is for config.
type Candidate struct {
	Row       Row
	Skip      string // non-empty: the row is left out, and why
	Name      Name
	Sex       string // "F" | "M" | ""
	BirthDate string // ISO, or ""
	BankRef   string // "Matrícula Interna" trimmed; students validates it
	Ordinal   int
	Warnings  []string
}

// PlacementKey is one (level, grade, group) of the file. Level is the name as
// the file first wrote it, spacing collapsed.
type PlacementKey struct {
	Level   string
	Ordinal int
	Group   string
}

// Planned is the whole file read: a candidate per row, the levels it names in
// order of appearance, and how many pupils each placement receives.
type Planned struct {
	Candidates []Candidate
	Levels     []string
	Placements map[PlacementKey]int
}

// Plan decides every row of the file.
func Plan(rows []Row) Planned {
	out := Planned{Placements: map[PlacementKey]int{}}
	levelByFold := map[string]string{}
	for _, r := range rows {
		c := candidate(r)
		if c.Skip == "" {
			level, seen := levelByFold[foldLevel(c.Row.LevelName)]
			if !seen {
				level = c.Row.LevelName
				levelByFold[foldLevel(c.Row.LevelName)] = level
				out.Levels = append(out.Levels, level)
			}
			out.Placements[PlacementKey{Level: level, Ordinal: c.Ordinal, Group: c.Row.GroupName}]++
		}
		out.Candidates = append(out.Candidates, c)
	}
	return out
}

// LevelOf is the name Plan keyed a row's level by, so a caller can map a
// candidate to its PlacementKey.
func (p Planned) LevelOf(c Candidate) string {
	for _, l := range p.Levels {
		if foldLevel(l) == foldLevel(c.Row.LevelName) {
			return l
		}
	}
	return c.Row.LevelName
}

func candidate(r Row) Candidate {
	r.GroupName = strings.ToUpper(strings.TrimSpace(r.GroupName))
	r.LevelName = collapse(r.LevelName)
	c := Candidate{Row: r, BankRef: strings.TrimSpace(r.LegacyRef)}
	switch strings.ToLower(Fold(r.Status)) {
	case statusActive:
	case statusWithdrawn:
		c.Skip = SkipWithdrawn
		return c
	default:
		c.Skip = SkipStatusUnknown
		return c
	}
	if r.LevelName == "" {
		c.Skip = SkipLevelMissing
		return c
	}
	ordinal, err := strconv.Atoi(strings.TrimSpace(r.Grade))
	if err != nil || ordinal < 1 {
		c.Skip = SkipGradeInvalid
		return c
	}
	c.Ordinal = ordinal
	if r.GroupName == "" || len(r.GroupName) > maxGroupLen {
		c.Skip = SkipGroupInvalid
		return c
	}
	name, err := SplitName(r.FullName, r.CURP)
	if err != nil {
		c.Skip = SkipNameUnreadable
		return c
	}
	c.Name = name
	c.Warnings = append(c.Warnings, name.Warnings...)
	c.Sex = sexFromCURP(r.CURP)
	c.BirthDate, c.Warnings = birthDate(r.BirthDate, r.CURP, c.Warnings)
	return c
}

// birthDate keeps the column's date when it reads and agrees with the CURP,
// and the CURP's otherwise — the CURP was issued from the birth certificate.
func birthDate(raw, curp string, warnings []string) (string, []string) {
	fromCURP := curpBirthDate(curp)
	typed, err := time.Parse("02/01/2006", strings.TrimSpace(raw))
	switch {
	case err == nil && (fromCURP.IsZero() || typed.Equal(fromCURP)):
		return typed.Format(time.DateOnly), warnings
	case !fromCURP.IsZero():
		return fromCURP.Format(time.DateOnly), append(warnings, WarnBirthDateFromCURP)
	default:
		return "", append(warnings, WarnBirthDateInvalid)
	}
}

// curpBirthDate reads positions 5–10 (YYMMDD); position 17 is a digit for
// those born in the 1900s and a letter from 2000 on.
func curpBirthDate(curp string) time.Time {
	c := strings.ToUpper(strings.TrimSpace(curp))
	if len(c) != curpLen {
		return time.Time{}
	}
	century := "20"
	if c[16] >= '0' && c[16] <= '9' {
		century = "19"
	}
	day, err := time.Parse("20060102", century+c[4:10])
	if err != nil {
		return time.Time{}
	}
	return day
}

// sexFromCURP reads position 11: H (hombre) → M, M (mujer) → F.
func sexFromCURP(curp string) string {
	c := strings.ToUpper(strings.TrimSpace(curp))
	if len(c) != curpLen {
		return ""
	}
	switch c[10] {
	case 'H':
		return "M"
	case 'M':
		return "F"
	}
	return ""
}

// foldLevel compares level names as config will: case, accents and spacing
// ignored.
func foldLevel(s string) string { return collapse(strings.ToLower(Fold(s))) }

// Package domain holds the rich domain model for the config bounded context (academic catalog: cycles, levels, grade_levels, subjects, terms, groups): entities, value objects, and domain (sentinel/typed) errors. It imports NO framework — not GORM, not Connect, not net/http. This is the innermost Clean Architecture layer; all dependencies point toward it.
package domain

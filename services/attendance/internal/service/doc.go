// Package service holds the attendance application logic (thin layout).
// Architecture is earned, not imposed: this layer stays small until attendance
// grows enough to warrant a full domain model.
//
// The model is DEFAULT-PRESENT: only exceptions (absent/late/excused) are
// stored, so an active enrollment with no record for a date IS present. Saving a
// day is therefore a reconcile — the caller sends the complete exception set for
// the group·day and the stored state is made to match it, which is what turns a
// dropped mark back into "present" without any explicit delete in the API.
//
// T3: a list is keyed by (group, date, subject) — an empty subject is the daily
// list, a subject a per-class list (the level's mode is enforced upstream, at
// the gateway). A save is diffed against what is stored (DiffList): rows are
// updated in place, and events go out only for what changed, so a correction
// never re-notifies the parents. Each save records the list's session, which is
// what tells an all-present day from a day nobody took.
//
// This package declares the seams it depends on (Repo, EnrollmentReader,
// GroupReader, Publisher) and imports NO framework: the GORM repository, the
// Connect reader clients and the Pub/Sub publisher all implement these
// interfaces from the outside.
package service

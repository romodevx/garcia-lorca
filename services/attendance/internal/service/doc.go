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
// This package declares the seams it depends on (Repo, EnrollmentReader,
// GroupReader, Publisher) and imports NO framework: the GORM repository, the
// Connect reader clients and the Pub/Sub publisher all implement these
// interfaces from the outside.
package service

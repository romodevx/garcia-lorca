// Shared libraries for all García Lorca Go services.
// Single module with one subpackage per directory (authctx, errors, logger,
// featureflag, validator, pubsub, pdf). Kept dependency-light on purpose: the
// foundational primitives here are imported by every service.
//
// The one deliberate exception is authctx/gormtx, which couples to GORM to
// adapt *gorm.DB to authctx.TxRunner — hoisted here because all six data
// services had a verbatim copy of it. authctx itself stays framework-free.
module github.com/garcia-lorca/pkg

go 1.24

require gorm.io/gorm v1.31.2

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/text v0.39.0 // indirect
)

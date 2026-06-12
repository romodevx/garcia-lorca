// Shared libraries for all García Lorca Go services.
// Single module with one subpackage per directory (authctx, errors, logger,
// featureflag, validator, pubsub, pdf). Kept dependency-light on purpose: the
// foundational primitives here are imported by every service.
module github.com/garcia-lorca/pkg

go 1.24

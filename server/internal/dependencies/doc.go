// Package dependencies manages typed dependency edges between services and
// accessories within an environment (spec §17, SH-037). Edges feed the
// topology graph and future configuration validation. Creating an edge that
// would introduce a service-to-service cycle is rejected.
package dependencies

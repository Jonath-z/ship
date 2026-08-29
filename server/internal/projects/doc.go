// Package projects owns the Project entity (spec §8): storage, validation, and
// HTTP handlers. A project is an application system such as "Shwary" or "Wavv".
//
// Layout convention used by every feature package in internal/:
//
//	service.go     business rules, the only place other packages call into
//	repository.go  SQL, nothing else
//	handler.go     HTTP transport, translates to/from the API contract
//	dto.go         request/response shapes tied to the OpenAPI spec
package projects

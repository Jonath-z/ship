// Package projects owns the Project entity (spec §8): storage, validation, and
// HTTP handlers. A project is an application system such as "Shwary" or "Wavv".
//
// Layout convention used by this feature package:
//
//	service.go     business rules and resource responses
//	repository.go  GORM persistence
//	routes.go      Gin transport and request DTOs
package projects

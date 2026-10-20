// Package domain holds the core entities, enums, business errors and shared
// value helpers for the ecommerce AI customer-support MVP.
//
// Layers above (service, tools, httpapi) must not depend on SQL specifics;
// this package must stay free of database, HTTP and LLM imports.
package domain

// Package render builds accessible server-rendered admin shells, resource
// screens, and guarded actions for workbench descriptors. Its HTML works
// without JavaScript and opts into GoSX managed navigation when enabled.
//
// The shell has a first-link skip target, named navigation landmarks, one
// main landmark, a status host, and a phone bar with at most four links. The
// main element can receive focus after managed navigation. Consumers put one
// page heading inside it. Lists preserve table headers when the theme stacks
// cells on narrow screens; detail screens use description lists. Forms link
// labels, help, and errors to controls, show a focused error summary after a
// server validation failure, and keep the GoSX managed-form status hooks.
//
// Renderers fail closed: unreadable destinations and hidden fields are
// omitted, and a nil Authorizer denies reads and actions. Consumers still
// enforce record-level scope in their stores.
package render

# GoSX Admin

GoSX Admin is the generic admin framework layer for the GoSX ecosystem.

## Agent Skill

Agents helping someone use GoSX Admin should read the GoSX ecosystem skill: [using-gosx-ecosystem](https://github.com/odvcencio/m31labs-skills/blob/main/skills/using-gosx-ecosystem/SKILL.md).

It owns reusable admin primitives: resource registries, generated surfaces,
server-action form wiring, and block-editor infrastructure. It should not know
about blog posts, products, or a specific CMS schema.

Core GoSX should stay focused on web primitives. This module is the opt-in
layer for admin surfaces that want typed Go definitions and tiny browser
bridges only where an interaction needs them.

Current package surface:

- `blockstudio`: sortable block definitions, persisted order normalization,
  form view models, generated inspector controls, and an embedded browser
  runtime for block lists.
- `workbench`: generic admin resource, field, action, tool, and workspace
  descriptors with map view models for GoSX admin shells.
- `calendar`: generic scheduling/event/resource/registration primitives,
  capacity helpers, month-grid view models, and rendered GoSX admin widgets.

```sh
go get github.com/odvcencio/gosx-admin
```

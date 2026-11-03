package buildinfo

// BaseVersion mirrors the canonical VERSION file via scripts/version.mjs.
const BaseVersion = "2.0.0"

// Builds without linker metadata are development builds, never release artifacts.
const DefaultVersion = BaseVersion + "-dev"

// Version is set once by the entrypoint, before any HTTP handlers start.
// Health and discovery endpoints report the same version as the binary banner.
var Version = DefaultVersion

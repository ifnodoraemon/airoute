package buildinfo

// BaseVersion is the next planned release. Keep frontend package metadata in sync.
const BaseVersion = "1.1.0"

// Builds without linker metadata are development builds, never release artifacts.
const DefaultVersion = BaseVersion + "-dev"

// Version is set once by the entrypoint, before any HTTP handlers start.
// Health and discovery endpoints report the same version as the binary banner.
var Version = DefaultVersion

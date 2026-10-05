package embed

// Windows reports writable regular files as 0666; Chmod only controls read-only.
const executablePermissions = 0o666

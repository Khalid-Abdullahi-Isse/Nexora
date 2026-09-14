package postgres

// Admin is retained as a source compatibility alias. Administrators are users
// with explicit roles/permissions, never a second credential table.
type Admin = User

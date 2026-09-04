package tea

// Bubble Tea v1 normally queries the default terminal background from init().
// Revolvr must complete its TTY gate before emitting terminal bytes, so its
// local v1.3.4 patch intentionally leaves package initialization inert.
